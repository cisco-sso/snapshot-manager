package manager

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"
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

func (r *reverts) ProcessSnapshotRevert(revert *vs.SnapshotRevert) error {
	glog.Infof("processing snapshot revert %v/%v %v\n%v", revert.Namespace, revert.Name, revert.Spec.TargetTime, revert)
	if revert.Spec.TargetTime != nil {
		if revert.Spec.StsType != nil {
			//optionally: validate
			sts, err := r.kube.GetSts(revert.Namespace, revert.Spec.StsType.Name)
			if err != nil {
				return e("Unable to get STS for %v revert", err, revert)
			}
			if sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
				return fmt.Errorf("STS for %v revert has %v replicas", revert, sts.Spec.Replicas)
			}
			selector, err := meta.LabelSelectorAsSelector(sts.Spec.Selector)
			if err != nil {
				return e("Unable to create label selector for revert %v", err, revert)
			}
			pvcs, err := r.kube.ListPVCs(selector)
			if err != nil {
				return e("Unable to list PVCs for revert %v", err, revert)
			}
			snapshots, err := r.getSnapshots(pvcs, revert.Spec.TargetTime)
			if err != nil {
				return e("Unable to create snapshots for revert %v", err, revert)
			}
			err = r.kube.SetStsReplica(sts.Namespace, sts.Name, 0)
			if err != nil {
				return e("Failed to scale down sts for revert %v", err, revert)
			}
			if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return r.kube.PodsDeleted(sts) }); err != nil {
				return e("waiting for pods deleted %v", err, sts)
			}
			defer func() {
				//TODO: do this with more effort to succeed
				r.kube.SetStsReplica(sts.Namespace, sts.Name, int(*sts.Spec.Replicas))
			}()
			for _, pvc := range pvcs {
				err := r.kube.DeletePVC(pvc)
				if err != nil {
					return e("Failed to delete pvc %v for revert %v", err, pvc, revert)
				}
			}
			for _, pvc := range pvcs {
				snapshotPvc := revert.AttachSnapshot(pvc, snapshots[pvc.Name].Metadata.Name)
				err := r.kube.CreatePVC(snapshotPvc)
				if err != nil {
					return e("Failed to create snapshot pvc %v for revert %v", err, snapshotPvc, revert)
				}
			}
			if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
				return r.kube.PvcsBound(pvcs), nil
			}); err != nil {
				return e("revert %v failed init, waiting for pvcs to bound", err, revert)
			}
		}
	}
	return nil
}

func (r *reverts) getSnapshots(pvcs []*core.PersistentVolumeClaim, targetTime *meta.Time) (map[string]*snap.VolumeSnapshot, error) {
	snaps, err := r.kube.ListSnapshots()
	if err != nil && len(snaps) < len(pvcs) {
		return nil, e("Failed finding all snapshots for pvcs", err)
	}
	snapMap := make(map[string]*snap.VolumeSnapshot)
	for _, snap := range snaps {
		if targetTime.Time.After(snap.Metadata.CreationTimestamp.Time) {
			if prev, ok := snapMap[snap.Spec.PersistentVolumeClaimName]; ok {
				if snap.Metadata.CreationTimestamp.Time.After(prev.Metadata.CreationTimestamp.Time) {
					snapMap[snap.Spec.PersistentVolumeClaimName] = snap
				}
			} else {
				snapMap[snap.Spec.PersistentVolumeClaimName] = snap
			}
		}
	}
	m := make(map[string]*snap.VolumeSnapshot)
	for _, pvc := range pvcs {
		if s, ok := snapMap[pvc.Name]; ok {
			m[pvc.Name] = s
		} else {
			return nil, fmt.Errorf("Missing snapshot for pvc %v/%v", pvc.Namespace, pvc.Name)
		}
	}
	return m, nil
}
