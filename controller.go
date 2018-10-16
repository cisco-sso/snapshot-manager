package main

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	svclientset "github.com/cisco-sso/snapshot-validator/pkg/client/clientset/versioned"
	svinformers "github.com/cisco-sso/snapshot-validator/pkg/client/informers/externalversions"
	vslister "github.com/cisco-sso/snapshot-validator/pkg/client/listers/snapshotvalidator/v1alpha1"
	"github.com/cisco-sso/snapshot-validator/pkg/validator"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"time"
)

type controller struct {
	kubeClient     kubernetes.Interface
	svClient       svclientset.Interface
	snapshotClient *restclient.RESTClient

	snapshotStore      kcache.Store
	snapshotController kcache.Controller

	stsInformer     kcache.SharedIndexInformer
	stsLister       appslisters.StatefulSetLister
	serviceInformer kcache.SharedIndexInformer
	serviceLister   corelisters.ServiceLister
	podInformer     kcache.SharedIndexInformer
	podLister       corelisters.PodLister
	pvcInformer     kcache.SharedIndexInformer
	pvcLister       corelisters.PersistentVolumeClaimLister
	pvInformer      kcache.SharedIndexInformer
	pvLister        corelisters.PersistentVolumeLister
	jobInformer     kcache.SharedIndexInformer
	jobLister       batchlisters.JobLister
	vsInformer      kcache.SharedIndexInformer
	vsLister        vslister.ValidationStrategyLister
	vrInformer      kcache.SharedIndexInformer
	vrLister        vslister.ValidationRunLister

	snapshotWorkqueue      workqueue.RateLimitingInterface
	validationRunWorkqueue workqueue.RateLimitingInterface
	validator              validator.Validator
}

func NewController(
	kubeClient kubernetes.Interface,
	snapshotClient *restclient.RESTClient,
	svClient svclientset.Interface,
	stopCh <-chan struct{}) *controller {
	syncTime := time.Second * 30

	kfac := kubeinformers.NewSharedInformerFactory(kubeClient, syncTime)
	svfac := svinformers.NewSharedInformerFactory(svClient, syncTime)
	c := &controller{
		kubeClient:     kubeClient,
		svClient:       svClient,
		snapshotClient: snapshotClient,

		stsInformer:     kfac.Apps().V1().StatefulSets().Informer(),
		stsLister:       kfac.Apps().V1().StatefulSets().Lister(),
		serviceInformer: kfac.Core().V1().Services().Informer(),
		serviceLister:   kfac.Core().V1().Services().Lister(),
		podInformer:     kfac.Core().V1().Pods().Informer(),
		podLister:       kfac.Core().V1().Pods().Lister(),
		pvcInformer:     kfac.Core().V1().PersistentVolumeClaims().Informer(),
		pvcLister:       kfac.Core().V1().PersistentVolumeClaims().Lister(),
		pvInformer:      kfac.Core().V1().PersistentVolumes().Informer(),
		pvLister:        kfac.Core().V1().PersistentVolumes().Lister(),
		jobInformer:     kfac.Batch().V1().Jobs().Informer(),
		jobLister:       kfac.Batch().V1().Jobs().Lister(),
		vsInformer:      svfac.Snapshotvalidator().V1alpha1().ValidationStrategies().Informer(),
		vsLister:        svfac.Snapshotvalidator().V1alpha1().ValidationStrategies().Lister(),
		vrInformer:      svfac.Snapshotvalidator().V1alpha1().ValidationRuns().Informer(),
		vrLister:        svfac.Snapshotvalidator().V1alpha1().ValidationRuns().Lister(),

		snapshotWorkqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "VolumeSnapshots"),
		validationRunWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ValidatorRun"),
		/*snapshotWorkqueue:      workqueue.NewNamedQueue("VolumeSnapshots"),
		validationRunWorkqueue: workqueue.NewNamedQueue("ValidatorRun"),*/
	}

	c.validator = validator.NewValidator(c)

	snapshotWatch := kcache.NewListWatchFromClient(
		snapshotClient,
		snap.VolumeSnapshotResourcePlural,
		core.NamespaceAll,
		fields.Everything())

	c.snapshotStore, c.snapshotController = kcache.NewInformer(
		snapshotWatch,
		&snap.VolumeSnapshot{},
		syncTime,
		kcache.ResourceEventHandlerFuncs{
			AddFunc: c.enqueueSnapshot,
		},
	)

	c.vrInformer.AddEventHandler(kcache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueValidatorRun,
		UpdateFunc: func(old, new interface{}) { c.enqueueValidatorRun(new) },
	})

	go kfac.Start(stopCh)
	go svfac.Start(stopCh)
	return c
}

func (c *controller) UpdateValidationRun(run *vs.ValidationRun) error {
	//TODO: make diff and then call patch
	var result vs.ValidationRun
	return c.svClient.SnapshotvalidatorV1alpha1().RESTClient().Put().
		Name(run.Name).
		Resource(vs.ValidationRunResourcePlural).
		Namespace(run.Namespace).
		Body(run).
		Do().Into(&result)
}
func (c *controller) CreateValidationRun(run *vs.ValidationRun) error {
	var result vs.ValidationRun
	return c.svClient.SnapshotvalidatorV1alpha1().RESTClient().Post().
		Resource(vs.ValidationRunResourcePlural).
		Namespace(run.Namespace).
		Body(run).
		Do().Into(&result)
}
func (c *controller) DeleteValidationRun(run *vs.ValidationRun) error {
	return c.svClient.SnapshotvalidatorV1alpha1().
		ValidationRuns(run.Namespace).
		Delete(run.Name, &meta.DeleteOptions{})
}
func (c *controller) CreatePVC(pvc *core.PersistentVolumeClaim) error {
	var result core.PersistentVolumeClaim
	return c.kubeClient.CoreV1().RESTClient().Post().
		Resource("persistentvolumeclaims").
		Namespace(pvc.Namespace).
		Body(pvc).
		Do().Into(&result)
}
func (c *controller) DeletePVC(pvc *core.PersistentVolumeClaim) error {
	return c.kubeClient.CoreV1().
		PersistentVolumeClaims(pvc.Namespace).
		Delete(pvc.Name, &meta.DeleteOptions{})
}
func (c *controller) CreateStatefulSet(sts *apps.StatefulSet) error {
	var result apps.StatefulSet
	return c.kubeClient.AppsV1().RESTClient().Post().
		Resource("statefulsets").
		Namespace(sts.Namespace).
		Body(sts).
		Do().Into(&result)
}
func (c *controller) CreateJob(job *batch.Job) error {
	var result batch.Job
	return c.kubeClient.BatchV1().RESTClient().Post().
		Resource("jobs").
		Namespace(job.Namespace).
		Body(job).
		Do().Into(&result)
}
func (c *controller) CreateService(svc *core.Service) error {
	var result core.Service
	return c.kubeClient.CoreV1().RESTClient().Post().
		Resource("services").
		Namespace(svc.Namespace).
		Body(svc).
		Do().Into(&result)
}
func (c *controller) LabelSnapshot(namespace, name, label, key string) error {
	//TODO: patch
	//patch := fmt.Sprintf(`[{"op":"replace","path":"/metadata/labels/%v","value":%v}]`, label, key)
	var result snap.VolumeSnapshot
	s, err := c.GetSnapshot(namespace, name)
	if err != nil {
		return fmt.Errorf("Failed getting snapshot %v/%v for relabel: %v", namespace, name)
	}
	copy := s.DeepCopy()
	copy.Metadata.Labels[label] = key
	return c.snapshotClient.Put().
		Resource(snap.VolumeSnapshotResourcePlural).
		Namespace(namespace).
		Name(name).
		Body(copy).
		Do().Into(&result)
}
func (c *controller) ListPods(selector labels.Selector) ([]*core.Pod, error) {
	return c.podLister.List(selector)
}
func (c *controller) ListStrategies() ([]*vs.ValidationStrategy, error) {
	return c.vsLister.List(labels.Everything())
}
func (c *controller) ListRuns() ([]*vs.ValidationRun, error) {
	return c.vrLister.List(labels.Everything())
}
func (c *controller) GetPVC(namespace, name string) (*core.PersistentVolumeClaim, error) {
	return c.pvcLister.PersistentVolumeClaims(namespace).Get(name)
}
func (c *controller) GetPV(name string) (*core.PersistentVolume, error) {
	return c.pvLister.Get(name)
}
func (c *controller) GetJob(namespace, name string) (*batch.Job, error) {
	if err := c.jobInformer.GetStore().Resync(); err != nil {
		glog.Error("Failed to resync jobsInformer store")
	}
	return c.jobLister.Jobs(namespace).Get(name)
}
func (c *controller) GetSts(namespace, name string) (*apps.StatefulSet, error) {
	return c.stsLister.StatefulSets(namespace).Get(name)
}
func (c *controller) GetSnapshot(namespace, name string) (*snap.VolumeSnapshot, error) {
	key := namespace + "/" + name
	obj, ok, err := c.snapshotStore.GetByKey(key)
	if !ok {
		return nil, fmt.Errorf("Snapshot %v not found", key)
	}
	if err != nil {
		return nil, fmt.Errorf("Finding Snapshot %v failed: %v", key, err)
	}
	snapshot, ok := obj.(*snap.VolumeSnapshot)
	if !ok {
		return nil, fmt.Errorf("Finding Snapshot %v failed typecast", key)
	}
	return snapshot, nil
}
func (c *controller) SetStsReplica(namespace, name string, replica int) error {
	patch := fmt.Sprintf(`[{"op":"replace","path":"/spec/replicas","value":%d}]`, replica)
	_, err := c.kubeClient.AppsV1().StatefulSets(namespace).
		Patch(name, types.JSONPatchType, []byte(patch))
	return err
}
func (c *controller) GetService(namespace, name string) (*core.Service, error) {
	return c.serviceLister.Services(namespace).Get(name)
}

func (c *controller) enqueueValidatorRun(obj interface{}) {
	vr, ok := obj.(*vs.ValidationRun)
	if !ok {
		glog.Warningf("expecting type ValidationRun but received type %T", obj)
		return
	}
	glog.Infof("enqueue ValidationRun %v/%v", vr.Namespace, vr.Name)

	if key, err := kcache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	} else {
		//TODO: learn how to properly sync cache
		time.Sleep(100 * time.Millisecond)
		c.validationRunWorkqueue.AddRateLimited(key)
	}
}

func (c *controller) enqueueSnapshot(obj interface{}) {
	snapshot, ok := obj.(*snap.VolumeSnapshot)
	if !ok {
		glog.Warningf("expecting type VolumeSnapshot but received type %T", obj)
		return
	}
	glog.Infof("enqueue snapshot %v/%v", snapshot.Metadata.Namespace, snapshot.Metadata.Name)

	if key, err := kcache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	} else {
		//TODO: learn how to properly sync cache
		time.Sleep(100 * time.Millisecond)
		c.snapshotWorkqueue.AddRateLimited(key)
	}
}

func (c *controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.snapshotWorkqueue.ShutDown()
	defer c.validationRunWorkqueue.ShutDown()
	glog.Info("Starting snapshot-validator controller")
	glog.Info("Waiting for informer caches to sync")
	go c.snapshotController.Run(stopCh)
	ok := kcache.WaitForCacheSync(stopCh,
		c.serviceInformer.HasSynced,
		c.snapshotController.HasSynced,
		c.podInformer.HasSynced,
		c.pvcInformer.HasSynced,
		c.jobInformer.HasSynced,
		c.vsInformer.HasSynced,
		c.vrInformer.HasSynced,
	)
	if !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	glog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(func() { c.runWorker(c.processNextSnapshot) }, time.Second, stopCh)
		go wait.Until(func() { c.runWorker(c.processNextValidation) }, time.Second, stopCh)
	}
	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")
	return nil
}

func (c *controller) runWorker(worker func() (bool, error)) {
	var err error
	var shutdown bool
	for !shutdown {
		shutdown, err = worker()
		if err != nil {
			runtime.HandleError(err)
		}
	}
}

func (c *controller) processNextValidation() (bool, error) {
	obj, shutdown := c.validationRunWorkqueue.Get()
	if shutdown {
		return shutdown, nil
	}
	defer c.validationRunWorkqueue.Done(obj)
	key, ok := obj.(string)
	if !ok {
		c.validationRunWorkqueue.Forget(obj)
		return false, fmt.Errorf("expected string in validation workqueue but got %#v", obj)
	}
	obj, ok, err := c.vrInformer.GetStore().GetByKey(key)
	if !ok || err != nil {
		c.validationRunWorkqueue.Forget(obj)
		return false, fmt.Errorf("key %v not found in store", key)
	}
	validationRun, ok := obj.(*vs.ValidationRun)
	if !ok {
		c.validationRunWorkqueue.Forget(obj)
		return false, fmt.Errorf("expected type ValidationRun for key %v but received type %T", key, obj)
	}
	if err := c.validator.ProcessValidationRun(validationRun); err != nil {
		return false, fmt.Errorf("processing validationRun %v failed: %v", key, err)
	}
	c.validationRunWorkqueue.Forget(obj)
	glog.Infof("Successfully synced '%s'", key)
	return false, nil
}

func (c *controller) processNextSnapshot() (bool, error) {
	obj, shutdown := c.snapshotWorkqueue.Get()
	if shutdown {
		return shutdown, nil
	}
	defer c.snapshotWorkqueue.Done(obj)
	key, ok := obj.(string)
	if !ok {
		c.snapshotWorkqueue.Forget(obj)
		return false, fmt.Errorf("expected string in workqueue but got %#v", obj)
	}
	obj, ok, err := c.snapshotStore.GetByKey(key)
	if !ok || err != nil {
		c.snapshotWorkqueue.Forget(obj)
		return false, fmt.Errorf("key %v not found in store", key)
	}
	snapshot, ok := obj.(*snap.VolumeSnapshot)
	if !ok {
		c.snapshotWorkqueue.Forget(obj)
		return false, fmt.Errorf("expected type VolumeSnapshot for key %v but received type %T", key, obj)
	}
	if err := c.validator.ProcessSnapshot(snapshot); err != nil {
		return false, fmt.Errorf("processing VolumeSnapshot %v failed: %v", key, err)
	}

	c.snapshotWorkqueue.Forget(obj)
	glog.Infof("Successfully synced '%s'", key)
	return false, nil
}
