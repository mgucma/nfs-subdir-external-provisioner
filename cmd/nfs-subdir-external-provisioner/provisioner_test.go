package main

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestResolveDeleteOptionsForProvision_PVCOverrides(t *testing.T) {
	provisioner := &nfsProvisioner{}
	storageClass := &storage.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "class"},
		Parameters: map[string]string{
			"onDelete":        "delete",
			"archiveOnDelete": "true",
		},
		ReclaimPolicy: func() *v1.PersistentVolumeReclaimPolicy {
			policy := v1.PersistentVolumeReclaimRetain
			return &policy
		}(),
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "claim",
			Annotations: map[string]string{
				pvcAnnotationOnDelete:        "retain",
				pvcAnnotationArchiveOnDelete: "false",
			},
		},
	}

	onDelete, archiveOnDelete, err := provisioner.resolveDeleteOptionsForProvision(controller.ProvisionOptions{
		PVC:          pvc,
		PVName:       "pv",
		StorageClass: storageClass,
	})
	if err != nil {
		t.Fatalf("resolveDeleteOptionsForProvision returned error: %v", err)
	}
	if onDelete != "retain" {
		t.Fatalf("expected onDelete=retain, got %q", onDelete)
	}
	if archiveOnDelete == nil || *archiveOnDelete {
		t.Fatalf("expected archiveOnDelete=false, got %v", archiveOnDelete)
	}
}

func TestResolveDeleteOptionsForProvision_Defaults(t *testing.T) {
	provisioner := &nfsProvisioner{
		defaultOnDelete:        "delete",
		defaultArchiveOnDelete: boolPtr(true),
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "claim",
		},
	}

	onDelete, archiveOnDelete, err := provisioner.resolveDeleteOptionsForProvision(controller.ProvisionOptions{
		PVC:    pvc,
		PVName: "pv",
		StorageClass: &storage.StorageClass{ReclaimPolicy: func() *v1.PersistentVolumeReclaimPolicy {
			policy := v1.PersistentVolumeReclaimRetain
			return &policy
		}()},
	})
	if err != nil {
		t.Fatalf("resolveDeleteOptionsForProvision returned error: %v", err)
	}
	if onDelete != "delete" {
		t.Fatalf("expected onDelete=delete, got %q", onDelete)
	}
	if archiveOnDelete == nil || !*archiveOnDelete {
		t.Fatalf("expected archiveOnDelete=true, got %v", archiveOnDelete)
	}
}

func TestResolveDeleteOptionsForProvision_InvalidValues(t *testing.T) {
	provisioner := &nfsProvisioner{}
	storageClass := &storage.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "class"},
		Parameters: map[string]string{
			"archiveOnDelete": "not-a-bool",
		},
		ReclaimPolicy: func() *v1.PersistentVolumeReclaimPolicy {
			policy := v1.PersistentVolumeReclaimDelete
			return &policy
		}(),
	}
	pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim"}}

	if _, _, err := provisioner.resolveDeleteOptionsForProvision(controller.ProvisionOptions{
		PVC:          pvc,
		PVName:       "pv",
		StorageClass: storageClass,
	}); err == nil {
		t.Fatalf("expected error when archiveOnDelete is invalid")
	}

	storageClass.Parameters = nil
	pvc.Annotations = map[string]string{pvcAnnotationArchiveOnDelete: "also-bad"}

	if _, _, err := provisioner.resolveDeleteOptionsForProvision(controller.ProvisionOptions{
		PVC:          pvc,
		PVName:       "pv",
		StorageClass: storageClass,
	}); err == nil {
		t.Fatalf("expected error when PVC annotation archive-on-delete is invalid")
	}
}

func TestResolveDeleteOptionsForVolume_PVAnnotationsOverride(t *testing.T) {
	provisioner := &nfsProvisioner{
		defaultOnDelete:        "delete",
		defaultArchiveOnDelete: boolPtr(true),
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				pvAnnotationOnDelete:        "retain",
				pvAnnotationArchiveOnDelete: "false",
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{NFS: &v1.NFSVolumeSource{Path: "/"}},
		},
	}

	onDelete, archiveOnDelete, err := provisioner.resolveDeleteOptionsForVolume(pv, &storage.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "class"},
		Parameters: map[string]string{
			"onDelete":        "delete",
			"archiveOnDelete": "true",
		},
	})
	if err != nil {
		t.Fatalf("resolveDeleteOptionsForVolume returned error: %v", err)
	}
	if onDelete != "retain" {
		t.Fatalf("expected onDelete=retain, got %q", onDelete)
	}
	if archiveOnDelete == nil || *archiveOnDelete {
		t.Fatalf("expected archiveOnDelete=false, got %v", archiveOnDelete)
	}
}

func TestResolveDeleteOptionsForVolume_Fallbacks(t *testing.T) {
	provisioner := &nfsProvisioner{
		defaultOnDelete:        "retain",
		defaultArchiveOnDelete: boolPtr(false),
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv"},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{NFS: &v1.NFSVolumeSource{Path: "/"}},
		},
	}
	storageClass := &storage.StorageClass{Parameters: map[string]string{"archiveOnDelete": "true"}}

	onDelete, archiveOnDelete, err := provisioner.resolveDeleteOptionsForVolume(pv, storageClass)
	if err != nil {
		t.Fatalf("resolveDeleteOptionsForVolume returned error: %v", err)
	}
	if onDelete != "retain" {
		t.Fatalf("expected onDelete=retain, got %q", onDelete)
	}
	if archiveOnDelete == nil || !*archiveOnDelete {
		t.Fatalf("expected archiveOnDelete=true, got %v", archiveOnDelete)
	}

	// Remove storage class parameter so the provisioner default is used.
	storageClass.Parameters = nil
	onDelete, archiveOnDelete, err = provisioner.resolveDeleteOptionsForVolume(pv, storageClass)
	if err != nil {
		t.Fatalf("resolveDeleteOptionsForVolume returned error after removing storage class param: %v", err)
	}
	if onDelete != "retain" {
		t.Fatalf("expected onDelete=retain, got %q", onDelete)
	}
	if archiveOnDelete == nil || *archiveOnDelete {
		t.Fatalf("expected archiveOnDelete=false from provisioner default, got %v", archiveOnDelete)
	}
}

func TestResolveDeleteOptionsForVolume_InvalidValues(t *testing.T) {
	provisioner := &nfsProvisioner{}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv",
			Annotations: map[string]string{
				pvAnnotationArchiveOnDelete: "invalid",
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{NFS: &v1.NFSVolumeSource{Path: "/"}},
		},
	}

	if _, _, err := provisioner.resolveDeleteOptionsForVolume(pv, nil); err == nil {
		t.Fatalf("expected error for invalid pv annotation")
	}

	pv.Annotations = nil
	storageClass := &storage.StorageClass{Parameters: map[string]string{"archiveOnDelete": "invalid"}}

	if _, _, err := provisioner.resolveDeleteOptionsForVolume(pv, storageClass); err == nil {
		t.Fatalf("expected error for invalid storage class archiveOnDelete")
	}
}
