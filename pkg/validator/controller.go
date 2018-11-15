package validator

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	svclientset "github.com/cisco-sso/snapshot-validator/pkg/client/clientset/versioned"
	svinformers "github.com/cisco-sso/snapshot-validator/pkg/client/informers/externalversions"
	vslister "github.com/cisco-sso/snapshot-validator/pkg/client/listers/snapshotvalidator/v1alpha1"
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

	//snapshotWorkqueue      workqueue.RateLimitingInterface
	//validationRunWorkqueue workqueue.RateLimitingInterface
	snapshotWorkqueue      workqueue.Interface
	validationRunWorkqueue workqueue.Interface
	validator              Validator
}

func NewController(clients Clients, stopCh <-chan struct{}) *controller {
	syncTime := time.Second * 30

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
		vsInformer:      svfac.Snapshotvalidator().V1alpha1().ValidationStrategies().Informer(),
		vsLister:        svfac.Snapshotvalidator().V1alpha1().ValidationStrategies().Lister(),
		vrInformer:      svfac.Snapshotvalidator().V1alpha1().ValidationRuns().Informer(),
		vrLister:        svfac.Snapshotvalidator().V1alpha1().ValidationRuns().Lister(),

		snapshotWorkqueue:      workqueue.NewNamed("VolumeSnapshots"),
		validationRunWorkqueue: workqueue.NewNamed("ValidatorRun"),
	}

	c.validator = NewValidator(c)

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
	})

	go kfac.Start(stopCh)
	go svfac.Start(stopCh)
	return c
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
		//c.validationRunWorkqueue.AddRateLimited(key)
		c.validationRunWorkqueue.Add(key)
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
		//c.snapshotWorkqueue.AddRateLimited(key)
		c.snapshotWorkqueue.Add(key)
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
		return false, fmt.Errorf("expected string in validation workqueue but got %#v", obj)
	}
	obj, ok, err := c.vrInformer.GetStore().GetByKey(key)
	if !ok || err != nil {
		return false, fmt.Errorf("key %v not found in store", key)
	}
	validationRun, ok := obj.(*vs.ValidationRun)
	if !ok {
		return false, fmt.Errorf("expected type ValidationRun for key %v but received type %T", key, obj)
	}
	if err := c.validator.ProcessValidationRun(validationRun); err != nil {
		defer c.validationRunWorkqueue.Add(key)
		return false, fmt.Errorf("processing validationRun %v failed: %v", key, err)
	}
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
		return false, fmt.Errorf("expected string in workqueue but got %#v", obj)
	}
	obj, ok, err := c.snapshotStore.GetByKey(key)
	if !ok || err != nil {
		return false, fmt.Errorf("key %v not found in store", key)
	}
	snapshot, ok := obj.(*snap.VolumeSnapshot)
	if !ok {
		return false, fmt.Errorf("expected type VolumeSnapshot for key %v but received type %T", key, obj)
	}
	if err := c.validator.ProcessSnapshot(snapshot); err != nil {
		defer c.snapshotWorkqueue.Add(key)
		return false, fmt.Errorf("processing VolumeSnapshot %v failed: %v", key, err)
	}

	glog.Infof("Successfully synced '%s'", key)
	return false, nil
}
