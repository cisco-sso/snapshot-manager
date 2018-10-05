package validator

import (
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
)

type KubeClient interface {
	ListStrategies() ([]*vs.ValidationStrategy, error)
	ListRuns() ([]*vs.ValidationRun, error)
}

type validator struct {
	queue      chan *snap.VolumeSnapshot
	kubeClient KubeClient
}

type Validator interface {
	Start(stopCh <-chan struct{})
	ProcessSnapshot(snapshot *snap.VolumeSnapshot) error
	ProcessValidationRun(run *vs.ValidationRun) error
}

func NewValidator(kubeClient KubeClient) Validator {
	return &validator{
		make(chan *snap.VolumeSnapshot, 10),
		kubeClient,
	}
}

func (v *validator) Start(stopCh <-chan struct{}) {
	<-stopCh
}

func (v *validator) matchStrategies(labels map[string]string) []vs.ValidationStrategy {
	//TODO: implement this
	return nil
}

/*func getRun(runs []vs.ValidationRun, snapshot *snap.VolumeSnapshot) {
	//find a run where this snapshot would fit
	//if no run found, start a new one
	strategies := matchStrategies(strategies, snapshot.Metadata.Labels)
	for _, s := range strategies {

	}
}*/

func (v *validator) ProcessValidationRun(run *vs.ValidationRun) error {
	glog.Info("processing validator run %v", run.Name)
	return nil
}
func (v *validator) ProcessSnapshot(snapshot *snap.VolumeSnapshot) error {
	glog.Info("processing snapshot %v", snapshot.Metadata.Name)
	//TODO:
	//group VolumeSnapshots by SnapshotValidation strategies
	//when SnapshotValidation strategy has all needed snapshots, run the validator
	// - template the strategy resources
	// - create PVCs from the strategy to point to snapshots
	// - create other resources from the strategy
	// - run query hook to the actual service
	// - run validate hook to the snapshot service
	// - teardown strategy resources
	glog.Info("finished processing snapshot %v", snapshot.Metadata.Name)
	return nil
}

/*func (v *validator) template(resources string, vals interface{}) (string, error) {
	//TODO:
	//tmpl, err := template.New("X").Parse(resources)
	//if err != nil {
	//	return "", err
	//}
	//var wr bytes.Buffer
	//if err := tmpl.Execute(wr, vals); err != nil {
	//	return "", err
	//}
	//return wr.String()
	return resources, nil
}*/
