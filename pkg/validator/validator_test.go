package validator

/*import (
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"testing"
)

type basicClient struct {
	validationRuns        map[string]*vs.ValidationRun
	newValidationRuns     []*vs.ValidationRun
	updatedValidationRuns []*vs.ValidationRun
	newPVCs               []*core.PersistentVolumeClaim
	newJobs               []*batch.Job
	newSts                []*apps.StatefulSet
}

func newBasicClient() *basicClient {
	return &basicClient{
		validationRuns: make(map[string]*vs.ValidationRun),
	}
}

func CreateService(*core.Service) error {
	return nil
}
func GetSts(string, string) (*apps.StatefulSet, error) {
	return nil, nil
}
func GetService(string, string) (*core.Service, error) {
	return nil, nil
}

func SetStsReplica(string, string, int) error {
	return
}
func (c *basicClient) ListStrategies() ([]*vs.ValidationStrategy, error) {
	pvc1 := core.PersistentVolumeClaim{}
	pvc1.Name = "snapshot-pvc1"
	pvc1.Namespace = "test"
	pvc2 := core.PersistentVolumeClaim{}
	pvc2.Name = "snapshot-pvc2"
	pvc2.Namespace = "test"
	init := batch.Job{}
	init.Name = "init"
	init.Namespace = "test"
	sts := apps.StatefulSet{}
	sts.Name = "snapshot-cassandra"
	sts.Namespace = "test"
	var replicas int32
	replicas = 2
	sts.Spec.Replicas = &replicas
	preval := batch.Job{}
	preval.Name = "preval"
	preval.Namespace = "test"
	val := batch.Job{}
	val.Name = "val"
	val.Namespace = "test"

	strategy := vs.ValidationStrategy{
		Spec: vs.ValidationStrategySpec{
			Selector: meta.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Claims: map[string]core.PersistentVolumeClaim{
				"pvc1": pvc1,
				"pvc2": pvc2,
			},
			Init:          &init,
			StatefulSet:   &sts,
			PreValidation: &preval,
			Validation:    &val,
		},
	}
	strategy.Name = "basic"
	strategy.Namespace = "test"
	return []*vs.ValidationStrategy{&strategy}, nil
}

func (c *basicClient) ListRuns() ([]*vs.ValidationRun, error) {
	runs := make([]*vs.ValidationRun, 0)
	for _, r := range c.validationRuns {
		runs = append(runs, r.DeepCopy())
	}
	return runs, nil
}
func (c *basicClient) ListPods(labels.Selector) ([]*core.Pod, error) {
	pod1 := core.Pod{}
	pod1.Name = "pod1"
	pod1.Namespace = "test"
	pod1.Labels = map[string]string{"app": "test"}
	pod1.Spec.Volumes = []core.Volume{
		{
			Name: "data",
		},
	}
	pod1.Spec.Volumes[0].PersistentVolumeClaim = &core.PersistentVolumeClaimVolumeSource{
		ClaimName: "pvc1",
	}
	pod1.Status.Phase = core.PodRunning
	pod2 := core.Pod{}
	pod2.Name = "pod2"
	pod2.Namespace = "test"
	pod2.Labels = map[string]string{"app": "test"}
	pod2.Spec.Volumes = []core.Volume{
		{
			Name: "data",
		},
	}
	pod2.Spec.Volumes[0].PersistentVolumeClaim = &core.PersistentVolumeClaimVolumeSource{
		ClaimName: "pvc2",
	}
	pod2.Status.Phase = core.PodRunning

	return []*core.Pod{&pod1, &pod2}, nil
}
func (c *basicClient) CreatePVC(pvc *core.PersistentVolumeClaim) error {
	c.newPVCs = append(c.newPVCs, pvc)
	return nil
}
func (c *basicClient) CreateValidationRun(v *vs.ValidationRun) error {
	c.newValidationRuns = append(c.newValidationRuns, v)
	c.validationRuns[v.Name] = v
	return nil
}
func (c *basicClient) CreateJob(j *batch.Job) error {
	c.newJobs = append(c.newJobs, j)
	return nil
}
func (c *basicClient) CreateStatefulSet(s *apps.StatefulSet) error {
	c.newSts = append(c.newSts, s)
	return nil
}
func (c *basicClient) UpdateValidationRun(v *vs.ValidationRun) error {
	c.updatedValidationRuns = append(c.updatedValidationRuns, v)
	c.validationRuns[v.Name] = v
	return nil
}
func (c *basicClient) GetPVC(namespace, name string) (*core.PersistentVolumeClaim, error) {
	pvc := core.PersistentVolumeClaim{}
	pvc.Name = name
	pvc.Namespace = namespace
	pvc.Status.Phase = core.ClaimBound
	return &pvc, nil
}
func (c *basicClient) GetJob(namespace, name string) (*batch.Job, error) {
	job := batch.Job{}
	job.Name = name
	job.Namespace = namespace
	job.Status.Succeeded = 1
	return &job, nil
}

func snapshot(name string) *snap.VolumeSnapshot {
	s := snap.VolumeSnapshot{}
	s.Metadata.Name = name + "-snapshot"
	s.Metadata.Namespace = "test"
	s.Spec.PersistentVolumeClaimName = name
	return &s
}

func TestBasicSnapshotPVC1(t *testing.T) {
	c := newBasicClient()
	v := NewValidator(c)
	s := snapshot("pvc1")
	err := v.ProcessSnapshot(s)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.newValidationRuns) != 1 {
		t.Fatal("Expected 1 new validation run but got", len(c.newValidationRuns))
	}
	if len(c.updatedValidationRuns) != 0 {
		t.Fatal("Expected 0 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run := c.newValidationRuns[0]
	if len(run.Spec.Snapshots) != 2 {
		t.Fatal("Expected 2 PVCs in snapshot run but got", len(run.Spec.Snapshots))
	}
	if run.Spec.Snapshots[s.Spec.PersistentVolumeClaimName] != s.Metadata.Name {
		t.Fatal("Expected run snapshot", s.Spec.PersistentVolumeClaimName,
			"to point to snapshot", s.Metadata.Name, "but is pointing to", run.Spec.Snapshots[s.Spec.PersistentVolumeClaimName])
	}
}

func TestBasicSnapshotPVC1and2(t *testing.T) {
	c := newBasicClient()
	v := NewValidator(c)
	s1 := snapshot("pvc1")
	err := v.ProcessSnapshot(s1)
	if err != nil {
		t.Error("Expected no error but got", err)
	}
	s2 := snapshot("pvc2")
	err = v.ProcessSnapshot(s2)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.newValidationRuns) != 1 {
		t.Fatal("Expected 1 new validation run but got", len(c.newValidationRuns))
	}
	if len(c.updatedValidationRuns) != 1 {
		t.Fatal("Expected 1 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run := c.updatedValidationRuns[0]
	if len(run.Spec.Snapshots) != 2 {
		t.Fatal("Expected 2 PVCs in snapshot run but got", len(run.Spec.Snapshots))
	}
	if run.Spec.Snapshots[s1.Spec.PersistentVolumeClaimName] != s1.Metadata.Name {
		t.Fatal("Expected run snapshot", s1.Spec.PersistentVolumeClaimName,
			"to point to snapshot", s1.Metadata.Name, "but is pointing to", run.Spec.Snapshots[s1.Spec.PersistentVolumeClaimName])
	}
	if run.Spec.Snapshots[s2.Spec.PersistentVolumeClaimName] != s2.Metadata.Name {
		t.Fatal("Expected run snapshot", s2.Spec.PersistentVolumeClaimName,
			"to point to snapshot", s2.Metadata.Name, "but is pointing to", run.Spec.Snapshots[s2.Spec.PersistentVolumeClaimName])
	}
}

func TestBasicFlow(t *testing.T) {
	c := newBasicClient()
	v := NewValidator(c)
	s1 := snapshot("pvc1")
	err := v.ProcessSnapshot(s1)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	err = v.ProcessValidationRun(c.newValidationRuns[0])
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	s2 := snapshot("pvc2")
	err = v.ProcessSnapshot(s2)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	err = v.ProcessValidationRun(c.updatedValidationRuns[0])
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.updatedValidationRuns) != 2 {
		t.Fatal("Expected 2 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run := c.updatedValidationRuns[1]
	if run.Status.InitStarted == nil || run.Status.InitFinished != nil {
		t.Fatal("Expected init to have started")
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[2]
	if run.Status.InitFinished == nil || run.Status.PreValidationStarted != nil {
		t.Fatal("Expected init to have finished")
	}
	if len(c.newPVCs) != 2 {
		t.Fatal("Expected 2 new PVCs but got", len(c.newPVCs))
	}
	if len(c.newJobs) != 1 {
		t.Fatal("Expected 1 init job but got", len(c.newJobs))
	}
	if len(c.newSts) != 1 {
		t.Fatal("Expected 1 stateful set but got", len(c.newSts))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[3]
	if run.Status.PreValidationStarted == nil || run.Status.PreValidationFinished != nil {
		t.Fatal("Expected pre-validation to have started")
	}
	if len(c.newJobs) != 1 {
		t.Fatal("Expected 1 init job but got", len(c.newJobs))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[4]
	if run.Status.PreValidationFinished == nil || run.Status.ValidationStarted != nil {
		t.Fatal("Expected pre-validation to have finished")
	}
	if len(c.newJobs) != 2 {
		t.Fatal("Expected 2 init job but got", len(c.newJobs))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[5]
	if run.Status.ValidationStarted == nil || run.Status.ValidationFinished != nil {
		t.Fatal("Expected validation to have started")
	}
	if len(c.newJobs) != 2 {
		t.Fatal("Expected 2 init job but got", len(c.newJobs))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[6]
	if run.Status.ValidationFinished == nil {
		t.Fatal("Expected validation to have finished")
	}
	if len(c.newJobs) != 3 {
		t.Fatal("Expected 3 init job but got", len(c.newJobs))
	}
}*/
