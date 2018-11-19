package validator

import (
	"bytes"
	"fmt"
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	"github.com/ghodss/yaml"
	"reflect"
	kust "sigs.k8s.io/kustomize/pkg/types"
	"sort"
	"strings"
	"testing"
	"text/tabwriter"
)

func defaultObjects() map[string]string {
	m := make(map[string]string)
	m["core/v1/Service/test"] = `apiVersion: v1
kind: Service
metadata:
  labels:
    app: test 
  name: test
  namespace: test-ns
spec:
  clusterIP: None
  ports:
  - name: port
    port: 6666
    protocol: TCP
    targetPort: 6666
  selector:
    app: test
  type: ClusterIP
`
	m["apps/v1/StatefulSet/test"] = `apiVersion: apps/v1
kind: StatefulSet
metadata:
  labels:
    app: test
  name: test
  namespace: test-ns
spec:
  selector:
    matchLabels:
      app: test
  serviceName: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - env:
        - name: env-change
          value: env
        image: img
        imagePullPolicy: IfNotPresent
        name: test 
        volumeMounts:
        - mountPath: /mount
          name: data
  volumeClaimTemplates:
  - metadata:
      creationTimestamp: null
      labels:
        app: cassandra
      name: data
    spec:
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 10Gi`
	return m
}

func defaultKust() vs.Kustomization {
	return vs.Kustomization{
		NamePrefix: "snapshot-",
		CommonLabels: map[string]string{
			"app": "snapshot-test",
		},
		Patches: map[string]string{
			"patch_sts": `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: test
  labels:
    app: test
spec:
  serviceName: snapshot-test
  template:
    spec:
      initContainers:
      - name: new-init
        image: new-image
        command:
        - bash
        - -c
        - 'echo "ok"'
      containers:
      - name: test
        env:
        - name: env-change 
          value: env-snapshot`,
		},
	}
}

func defaultOut() string {
	return `apiVersion: v1
kind: Service
metadata:
  labels:
    app: snapshot-test
  name: snapshot-test
  namespace: test-ns
spec:
  clusterIP: None
  ports:
  - name: port
    port: 6666
    protocol: TCP
    targetPort: 6666
  selector:
    app: snapshot-test
  type: ClusterIP
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  labels:
    app: snapshot-test
  name: snapshot-test
  namespace: test-ns
spec:
  selector:
    matchLabels:
      app: snapshot-test
  serviceName: snapshot-test
  template:
    metadata:
      labels:
        app: snapshot-test
    spec:
      containers:
      - env:
        - name: env-change
          value: env-snapshot
        image: img
        imagePullPolicy: IfNotPresent
        name: test
        volumeMounts:
        - mountPath: /mount
          name: data
      initContainers:
      - command:
        - bash
        - -c
        - echo "ok"
        image: new-image
        name: new-init
  volumeClaimTemplates:
  - metadata:
      creationTimestamp: null
      labels:
        app: snapshot-test
      name: data
    spec:
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 10Gi
`
}

func defaultKustOut() []byte {
	return []byte(`resources:
- core/v1/Service/test.yaml
- apps/v1/StatefulSet/test.yaml
commonLabels:
  app: snapshot-test
namePrefix: snapshot-
patchesStrategicMerge:
- patch_sts.yaml`)
}

func column(a, b string) string {
	alines, blines := strings.Split(a, "\n"), strings.Split(b, "\n")
	out := bytes.NewBufferString("")
	w := tabwriter.NewWriter(out, 0, 0, 1, ' ', tabwriter.Debug)
	for i := 0; i < len(alines) || i < len(blines); i++ {
		if len(alines) > i {
			fmt.Fprint(w, alines[i])
		}
		fmt.Fprint(w, "\t")
		if len(blines) > i {
			fmt.Fprint(w, blines[i])
		}
		fmt.Fprint(w, "\n")
	}
	w.Flush()
	return out.String()
}

func sortArrays(k *kust.Kustomization) {
	//TODO: may need to sort other arrays when starting to use them
	sort.Strings(k.Resources)
}

func kustEqual(a, b []byte) error {
	ak := kust.Kustomization{}
	err := yaml.Unmarshal(a, &ak)
	if err != nil {
		return fmt.Errorf("ak: %v", err)
	}
	bk := kust.Kustomization{}
	err = yaml.Unmarshal(b, &bk)
	if err != nil {
		return fmt.Errorf("bk: %v", err)
	}
	sortArrays(&ak)
	sortArrays(&bk)
	if !reflect.DeepEqual(ak, bk) {
		out := column(string(a), string(b))
		ayaml, _ := yaml.Marshal(ak)
		byaml, _ := yaml.Marshal(bk)
		outm := column(string(ayaml), string(byaml))
		return fmt.Errorf("[original diff]\n%v\n[marshaled diff]\n%v", out, outm)
	}
	return nil
}

func TestKustomization(t *testing.T) {
	//fakefs init
	fs, err := initFakeFs("/abc-run-test", defaultObjects(), defaultKust())
	if err != nil {
		t.Fatal(err)
	}
	out, err := fs.ReadFile("/abc-run-test/kustomization.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := kustEqual(out, defaultKustOut()); err != nil {
		t.Fatal(err)
	}

	//kustomization
	kOut := bytes.NewBufferString("")
	if err = runKustomizeBuild("/abc-run-test", kOut, fs); err != nil {
		t.Fatal(err)
	}
	expect := defaultOut()
	got := kOut.String()
	if expect != got {
		cOut := column(expect, got)
		t.Fatal(fmt.Errorf("[expect | got]\n%v", cOut))
	}
	//pvcs

}
