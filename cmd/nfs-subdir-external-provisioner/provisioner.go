/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"

	storage "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller"
)

const (
	provisionerNameKey           = "PROVISIONER_NAME"
	envArchiveOnDelete           = "PROVISIONER_ARCHIVE_ON_DELETE"
	envOnDelete                  = "PROVISIONER_ON_DELETE"
	pvcAnnotationArchiveOnDelete = "nfs.io/archive-on-delete"
	pvcAnnotationOnDelete        = "nfs.io/on-delete"
	pvAnnotationArchiveOnDelete  = "nfs.io/archive-on-delete"
	pvAnnotationOnDelete         = "nfs.io/on-delete"
)

type nfsProvisioner struct {
	client                 kubernetes.Interface
	server                 string
	path                   string
	defaultArchiveOnDelete *bool
	defaultOnDelete        string
}

type pvcMetadata struct {
	data        map[string]string
	labels      map[string]string
	annotations map[string]string
}

var pattern = regexp.MustCompile(`\${\.PVC\.((labels|annotations)\.(.*?)|.*?)}`)

func (meta *pvcMetadata) stringParser(str string) string {
	result := pattern.FindAllStringSubmatch(str, -1)
	for _, r := range result {
		switch r[2] {
		case "labels":
			str = strings.ReplaceAll(str, r[0], meta.labels[r[3]])
		case "annotations":
			str = strings.ReplaceAll(str, r[0], meta.annotations[r[3]])
		default:
			str = strings.ReplaceAll(str, r[0], meta.data[r[1]])
		}
	}

	return str
}

const (
	mountPath = "/persistentvolumes"
)

var _ controller.Provisioner = &nfsProvisioner{}

func normalizeOnDelete(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "delete":
		return "delete"
	case "retain":
		return "retain"
	default:
		return ""
	}
}

func (p *nfsProvisioner) resolveDeleteOptionsForProvision(options controller.ProvisionOptions) (string, *bool, error) {
	var onDelete string
	if options.StorageClass != nil {
		onDelete = normalizeOnDelete(options.StorageClass.Parameters["onDelete"])
	}
	if annotation := options.PVC.Annotations[pvcAnnotationOnDelete]; annotation != "" {
		if normalized := normalizeOnDelete(annotation); normalized != "" {
			onDelete = normalized
		}
	}
	if onDelete == "" {
		onDelete = normalizeOnDelete(p.defaultOnDelete)
	}

	var archiveOnDelete *bool
	if options.StorageClass != nil {
		if value, exists := options.StorageClass.Parameters["archiveOnDelete"]; exists {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return "", nil, fmt.Errorf("failed to parse archiveOnDelete parameter: %w", err)
			}
			archiveOnDelete = &parsed
		}
	}

	if value, exists := options.PVC.Annotations[pvcAnnotationArchiveOnDelete]; exists && value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", nil, fmt.Errorf("failed to parse %s annotation: %w", pvcAnnotationArchiveOnDelete, err)
		}
		archiveOnDelete = &parsed
	} else if archiveOnDelete == nil && p.defaultArchiveOnDelete != nil {
		defaultValue := *p.defaultArchiveOnDelete
		archiveOnDelete = &defaultValue
	}

	return onDelete, archiveOnDelete, nil
}

func (p *nfsProvisioner) resolveDeleteOptionsForVolume(volume *v1.PersistentVolume, storageClass *storage.StorageClass) (string, *bool, error) {
	var onDelete string
	if volume.Annotations != nil {
		if value := normalizeOnDelete(volume.Annotations[pvAnnotationOnDelete]); value != "" {
			onDelete = value
		}
	}
	if onDelete == "" && storageClass != nil {
		onDelete = normalizeOnDelete(storageClass.Parameters["onDelete"])
	}
	if onDelete == "" {
		onDelete = normalizeOnDelete(p.defaultOnDelete)
	}

	var archiveOnDelete *bool
	if volume.Annotations != nil {
		if value, exists := volume.Annotations[pvAnnotationArchiveOnDelete]; exists && value != "" {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return "", nil, fmt.Errorf("failed to parse %s annotation on PV %s: %w", pvAnnotationArchiveOnDelete, volume.Name, err)
			}
			archiveOnDelete = &parsed
		}
	}
	if archiveOnDelete == nil && storageClass != nil {
		if value, exists := storageClass.Parameters["archiveOnDelete"]; exists {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return "", nil, fmt.Errorf("failed to parse archiveOnDelete parameter on StorageClass %s: %w", storageClass.Name, err)
			}
			archiveOnDelete = &parsed
		} else if p.defaultArchiveOnDelete != nil {
			defaultValue := *p.defaultArchiveOnDelete
			archiveOnDelete = &defaultValue
		}
	} else if archiveOnDelete == nil && p.defaultArchiveOnDelete != nil {
		defaultValue := *p.defaultArchiveOnDelete
		archiveOnDelete = &defaultValue
	}

	return onDelete, archiveOnDelete, nil
}

func (p *nfsProvisioner) Provision(ctx context.Context, options controller.ProvisionOptions) (*v1.PersistentVolume, controller.ProvisioningState, error) {
	if options.PVC.Spec.Selector != nil {
		return nil, controller.ProvisioningFinished, fmt.Errorf("claim Selector is not supported")
	}
	glog.V(4).Infof("nfs provisioner: VolumeOptions %v", options)

	pvcNamespace := options.PVC.Namespace
	pvcName := options.PVC.Name

	pvName := strings.Join([]string{pvcNamespace, pvcName, options.PVName}, "-")

	metadata := &pvcMetadata{
		data: map[string]string{
			"name":      pvcName,
			"namespace": pvcNamespace,
		},
		labels:      options.PVC.Labels,
		annotations: options.PVC.Annotations,
	}

	fullPath := filepath.Join(mountPath, pvName)
	path := filepath.Join(p.path, pvName)

	pathPattern, exists := options.StorageClass.Parameters["pathPattern"]
	if exists {
		customPath := strings.TrimSpace(metadata.stringParser(pathPattern))
		if customPath != "" {
			customPath = filepath.Clean(customPath)

			if filepath.IsAbs(customPath) {
				customPath = strings.TrimPrefix(customPath, string(filepath.Separator))
			}

			switch customPath {
			case "", ".":
				customPath = ""
			}
		}

		if customPath != "" && !strings.HasPrefix(customPath, "..") {
			path = filepath.Join(p.path, customPath)
			fullPath = filepath.Join(mountPath, customPath)
		}
	}

	glog.V(4).Infof("creating path %s", fullPath)
	if err := os.MkdirAll(fullPath, 0o777); err != nil {
		return nil, controller.ProvisioningFinished, errors.New("unable to create directory to provision new pv: " + err.Error())
	}
	if err := os.Chmod(fullPath, 0o777); err != nil {
		if removeErr := os.RemoveAll(fullPath); removeErr != nil {
			glog.Warningf("unable to clean up path %s after chmod error: %v", fullPath, removeErr)
		}
		return nil, controller.ProvisioningFinished, fmt.Errorf("unable to set permissions on %s: %w", fullPath, err)
	}

	onDelete, archiveOnDelete, err := p.resolveDeleteOptionsForProvision(options)
	if err != nil {
		if removeErr := os.RemoveAll(fullPath); removeErr != nil {
			glog.Warningf("unable to clean up path %s after delete options error: %v", fullPath, removeErr)
		}
		return nil, controller.ProvisioningFinished, err
	}

	annotations := map[string]string{}
	if onDelete != "" {
		annotations[pvAnnotationOnDelete] = onDelete
	}
	if archiveOnDelete != nil {
		annotations[pvAnnotationArchiveOnDelete] = strconv.FormatBool(*archiveOnDelete)
	}

	if len(annotations) == 0 {
		annotations = nil
	}

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        options.PVName,
			Annotations: annotations,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: *options.StorageClass.ReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			MountOptions:                  options.StorageClass.MountOptions,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				NFS: &v1.NFSVolumeSource{
					Server:   p.server,
					Path:     path,
					ReadOnly: false,
				},
			},
		},
	}
	return pv, controller.ProvisioningFinished, nil
}

func (p *nfsProvisioner) Delete(ctx context.Context, volume *v1.PersistentVolume) error {
	path := filepath.Clean(volume.Spec.PersistentVolumeSource.NFS.Path)
	basePath := filepath.Base(path)

	serverPath := filepath.Clean(p.path)
	relPath, err := filepath.Rel(serverPath, path)
	if err != nil {
		return fmt.Errorf("failed to resolve relative path for %s: %w", path, err)
	}
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("invalid path %s for server root %s", path, serverPath)
	}

	oldPath := filepath.Join(mountPath, relPath)

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		glog.Warningf("path %s does not exist, deletion skipped", oldPath)
		return nil
	}
	// Get the storage class for this volume.
	storageClass, err := p.getClassForVolume(ctx, volume)
	if err != nil {
		return err
	}

	onDelete, archiveOnDelete, err := p.resolveDeleteOptionsForVolume(volume, storageClass)
	if err != nil {
		return err
	}

	// Determine if the "onDelete" option exists.
	// If it exists and has a `delete` value, delete the directory.
	// If it exists and has a `retain` value, save the directory.
	switch onDelete {
	case "delete":
		return os.RemoveAll(oldPath)
	case "retain":
		return nil
	}

	// Determine if the "archiveOnDelete" option exists.
	// If it exists and has a false value, delete the directory.
	// Otherwise, archive it.
	if archiveOnDelete != nil && !*archiveOnDelete {
		return os.RemoveAll(oldPath)
	}

	archivePath := filepath.Join(mountPath, "archived-"+basePath)
	glog.V(4).Infof("archiving path %s to %s", oldPath, archivePath)
	return os.Rename(oldPath, archivePath)
}

// getClassForVolume returns StorageClass.
func (p *nfsProvisioner) getClassForVolume(ctx context.Context, pv *v1.PersistentVolume) (*storage.StorageClass, error) {
	if p.client == nil {
		return nil, fmt.Errorf("cannot get kube client")
	}
	className := storagehelpers.GetPersistentVolumeClass(pv)
	if className == "" {
		return nil, fmt.Errorf("volume has no storage class")
	}
	class, err := p.client.StorageV1().StorageClasses().Get(ctx, className, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return class, nil
}

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")

	server := os.Getenv("NFS_SERVER")
	if server == "" {
		glog.Fatal("NFS_SERVER not set")
	}
	path := os.Getenv("NFS_PATH")
	if path == "" {
		glog.Fatal("NFS_PATH not set")
	}
	provisionerName := os.Getenv(provisionerNameKey)
	if provisionerName == "" {
		glog.Fatalf("environment variable %s is not set! Please set it.", provisionerNameKey)
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	var config *rest.Config
	if kubeconfig != "" {
		// Create an OutOfClusterConfig and use it to create a client for the controller
		// to use to communicate with Kubernetes
		var err error
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			glog.Fatalf("Failed to create kubeconfig: %v", err)
		}
	} else {
		// Create an InClusterConfig and use it to create a client for the controller
		// to use to communicate with Kubernetes
		var err error
		config, err = rest.InClusterConfig()
		if err != nil {
			glog.Fatalf("Failed to create config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		glog.Fatalf("Error getting server version: %v", err)
	}

	leaderElection := true
	leaderElectionEnv := os.Getenv("ENABLE_LEADER_ELECTION")
	if leaderElectionEnv != "" {
		leaderElection, err = strconv.ParseBool(leaderElectionEnv)
		if err != nil {
			glog.Fatalf("Unable to parse ENABLE_LEADER_ELECTION env var: %v", err)
		}
	}

	var defaultArchiveOnDelete *bool
	if value := os.Getenv(envArchiveOnDelete); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			glog.Fatalf("Unable to parse %s env var: %v", envArchiveOnDelete, err)
		}
		defaultArchiveOnDelete = &parsed
	}

	defaultOnDelete := ""
	if value := os.Getenv(envOnDelete); value != "" {
		normalized := normalizeOnDelete(value)
		if normalized == "" {
			glog.Fatalf("Invalid value for %s env var: %s", envOnDelete, value)
		}
		defaultOnDelete = normalized
	}

	clientNFSProvisioner := &nfsProvisioner{
		client:                 clientset,
		server:                 server,
		path:                   path,
		defaultArchiveOnDelete: defaultArchiveOnDelete,
		defaultOnDelete:        defaultOnDelete,
	}
	// Start the provision controller which will dynamically provision efs NFS
	// PVs
	pc := controller.NewProvisionController(clientset,
		provisionerName,
		clientNFSProvisioner,
		serverVersion.GitVersion,
		controller.LeaderElection(leaderElection),
	)
	// Never stops.
	pc.Run(context.Background())
}
