# Dependency and tooling refresh overview

This note summarises the practical benefits of the dependency and automation
updates introduced in the latest maintenance work.

## Go module updates

- **github.com/golang/glog v1.2.4** – keeps the provisioner on the
  most recent upstream logging fixes so we inherit improvements around
  flag handling and log flushing behaviour that have landed since the
  previous tag.
- **golang.org/x/net v0.23.0** – pulls in the current batch of security
  and robustness patches for the Go networking stack, including fixes for
  HTTP/2 request handling and DNS resolver edge-cases that were not
  available in the older release.

## Build tooling

- **tonistiigi/xx 1.6.1** – updates the cross-compilation environment used by
  the multi-architecture Docker build so we can target the latest
  toolchains and receive upstream QEMU fixes.

## CI / CD automation

- **actions/checkout v4** – moves the GitHub workflows onto the Node 20 based
  runner, ensuring long-term support from GitHub Actions.
- **docker actions** – the login, setup-buildx, and build-push actions are kept
  on their current major releases, which include compatibility fixes for
  the latest Docker Engine versions.

These upgrades are routine, but together they keep the project aligned with
actively maintained tooling and hardened upstream libraries.
