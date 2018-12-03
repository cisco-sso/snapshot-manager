package manager

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
	svclientset "github.com/cisco-sso/snapshot-manager/pkg/client/clientset/versioned"
	svinformers "github.com/cisco-sso/snapshot-manager/pkg/client/informers/externalversions"
	vslister "github.com/cisco-sso/snapshot-manager/pkg/client/listers/snapshotmanager/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
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

type Clients struct {
	KubeClientset  kubernetes.Interface
	SnapshotClient *restclient.RESTClient
	SvClientset    svclientset.Interface
}

type controller struct {
	clients Clients

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
	srInformer      kcache.SharedIndexInformer
	srLister        vslister.SnapshotRevertLister

	snapshotWorkqueue      workqueue.RateLimitingInterface
	validationRunWorkqueue workqueue.RateLimitingInterface
	revertWorkqueue        workqueue.RateLimitingInterface

	reverts   Reverts
	validator Validator
}

func NewController(clients Clients, stopCh <-chan struct{}) *controller {
	syncTime := time.Hour

	kfac := kubeinformers.NewSharedInformerFactory(clients.KubeClientset, syncTime)
	svfac := svinformers.NewSharedInformerFactory(clients.SvClientset, syncTime)
	c := &controller{
		clients: clients,

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
		vsInformer:      svfac.Snapshotmanager().V1alpha1().ValidationStrategies().Informer(),
		vsLister:        svfac.Snapshotmanager().V1alpha1().ValidationStrategies().Lister(),
		vrInformer:      svfac.Snapshotmanager().V1alpha1().ValidationRuns().Informer(),
		vrLister:        svfac.Snapshotmanager().V1alpha1().ValidationRuns().Lister(),
		srInformer:      svfac.Snapshotmanager().V1alpha1().SnapshotReverts().Informer(),
		srLister:        svfac.Snapshotmanager().V1alpha1().SnapshotReverts().Lister(),

		snapshotWorkqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "VolumeSnapshots"),
		validationRunWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ValidatorRun"),
		revertWorkqueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "SnapshotRevert"),
	}

	c.validator = NewValidator(c)
	c.reverts = NewReverts(c)

	snapshotWatch := kcache.NewListWatchFromClient(
		clients.SnapshotClient,
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
		DeleteFunc: c.scheduleValidatorRunDelete,
	})
	c.srInformer.AddEventHandler(kcache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueSnapshotRevert,
		UpdateFunc: func(old, new interface{}) { c.enqueueSnapshotRevert(new) },
	})

	go kfac.Start(stopCh)
	go svfac.Start(stopCh)
	return c
}

func (c *controller) enqueueSnapshotRevert(obj interface{}) {
	vr, ok := obj.(*vs.SnapshotRevert)
	if !ok {
		glog.Warningf("expecting type SnapshotRevert but received type %T", obj)
		return
	}
	glog.Infof("enqueue SnapshotRevert %v/%v", vr.Namespace, vr.Name)

	if key, err := kcache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	} else {
		c.revertWorkqueue.Add(key)
	}
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
		c.validationRunWorkqueue.Add(key)
	}
}

func (c *controller) scheduleValidatorRunDelete(obj interface{}) {
	var run *vs.ValidationRun
	var ok bool
	if run, ok = obj.(*vs.ValidationRun); !ok {
		tombstone, ok := obj.(kcache.DeletedFinalStateUnknown)
		if !ok {
			glog.Errorf("decoding run, invalid type")
			return
		}
		if run, ok = tombstone.Obj.(*vs.ValidationRun); !ok {
			glog.Errorf("decoding run tombstone, invalid type")
			return
		}
		glog.V(4).Infof("Recovered deleted run '%s' from tombstone", run.GetName())
	}
	c.validator.ProcessValidationRunDelete(run)
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
		c.snapshotWorkqueue.Add(key)
	}
}

func (c *controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.snapshotWorkqueue.ShutDown()
	defer c.validationRunWorkqueue.ShutDown()
	glog.Info("Starting snapshot-manager controller")
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
		c.srInformer.HasSynced,
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
		c.validationRunWorkqueue.AddRateLimited(key)
		return false, fmt.Errorf("processing validationRun %v failed: %v", key, err)
	}
	c.validationRunWorkqueue.Forget(key)
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
		c.snapshotWorkqueue.AddRateLimited(key)
		return false, fmt.Errorf("processing VolumeSnapshot %v failed: %v", key, err)
	}

	glog.Infof("Successfully synced '%s'", key)
	return false, nil
}
