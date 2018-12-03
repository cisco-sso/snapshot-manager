package manager

import (
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
)

type reverts struct {
	kube KubeCalls
}

type Reverts interface {
	ProcessSnapshotRevert(snapshotRevert *vs.SnapshotRevert) error
}

func NewReverts(kube KubeCalls) Reverts {
	return &reverts{
		kube,
	}
}

func (r *reverts) ProcessSnapshotRevert(snapshotRevert *vs.SnapshotRevert) error {
	return nil
}
