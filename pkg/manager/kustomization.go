package manager

import (
	"bytes"
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
	"github.com/ghodss/yaml"
	"io"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/kustomize/k8sdeps"
	kustbuild "sigs.k8s.io/kustomize/pkg/commands/build"
	kustfs "sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/patch"
	kust "sigs.k8s.io/kustomize/pkg/types"
	"strings"
)

func (v *validator) getYamlObjectMap(strategy *vs.ValidationStrategy) (map[string]string, error) {
	yamlMap := make(map[string]string)
	for _, r := range strategy.GetKustResources() {
		if obj, err := v.kube.GetUnstructuredObject(strategy.Namespace, r); err != nil {
			return nil, e("failed to get object %v", err, r.Id())
		} else {
			bytes, err := obj.MarshalJSON()
			if err != nil {
				return nil, e("failed to marshal %v", err, r.Id())
			}
			yamlBytes, err := yaml.JSONToYAML(bytes)
			if err != nil {
				return nil, e("failed to convert json to yaml %v", err, r.Id())
			}
			yamlMap[r.Id()] = string(yamlBytes)
		}
	}
	return yamlMap, nil
}

func initFakeFs(fsName string, objects map[string]string, kustomization vs.Kustomization) (kustfs.FileSystem, error) {
	fs := kustfs.MakeFakeFS()
	if err := fs.MkdirAll(fsName); err != nil {
		return nil, e("Failed to create root dir", err)
	}
	k := kust.Kustomization{
		NamePrefix:        kustomization.NamePrefix,
		CommonLabels:      kustomization.CommonLabels,
		CommonAnnotations: kustomization.CommonAnnotations,
	}
	for name, obj := range objects {
		f, err := fs.Create(fsName + "/" + name + ".yaml")
		if err != nil {
			return nil, e("Failed to create %v", err, name)
		}
		if _, err = f.Write([]byte(obj)); err != nil {
			return nil, e("Failed to write %v", err, name)
		}
		k.Resources = append(k.Resources, name+".yaml")
	}
	for name, obj := range kustomization.Patches {
		f, err := fs.Create(fsName + "/" + name + ".yaml")
		if err != nil {
			return nil, e("Failed to create %v", err, name)
		}
		if _, err = f.Write([]byte(obj)); err != nil {
			return nil, e("Failed to write %v", err, name)
		}
		k.PatchesStrategicMerge = append(k.PatchesStrategicMerge, patch.StrategicMerge(name+".yaml"))
	}
	f, err := fs.Create(fsName + "/kustomization.yaml")
	if err != nil {
		return nil, e("Failed to create kustomization.yaml", err)
	}
	kyaml, err := yaml.Marshal(k)
	if err != nil {
		return nil, e("Failed to marshal kustomization.yaml", err)
	}
	_, err = f.Write(kyaml)
	if err != nil {
		return nil, e("Failed to write kustomization.yaml", err)
	}
	return fs, nil
}

func runKustomizeBuild(path string, out io.Writer, fs kustfs.FileSystem) error {
	o := kustbuild.NewBuildOptions(path)
	f := k8sdeps.NewFactory()
	return o.RunBuild(out, fs, f.ResmapF, f.TransformerF)
}

func (v *validator) getClaims(run *vs.ValidationRun) (map[string]*core.PersistentVolumeClaim, error) {
	claims := make(map[string]*core.PersistentVolumeClaim)
	for c, snap := range run.Spec.ClaimsToSnapshots {
		claim, err := v.kube.GetPVC(run.Namespace, c)
		if err != nil {
			return claims, err
		}
		claims[snap] = claim
	}
	return claims, nil
}

func (v *validator) kustomize(strategy *vs.ValidationStrategy, run *vs.ValidationRun) error {
	yaml, err := v.getYamlObjectMap(strategy)
	if err != nil {
		return e("failed to get object map", err)
	}
	root := "/" + run.Name
	fs, err := initFakeFs(root, yaml, strategy.Spec.Kustomization)
	if err != nil {
		return e("failed to initFakeFs", err)
	}
	out := bytes.NewBufferString("")
	if err = runKustomizeBuild(root, out, fs); err != nil {
		return e("failed to run kustomize", err)
	}
	outObjects := strings.Split(out.String(), "---")
	run.Spec.Objects.Kustomized = append(run.Spec.Objects.Kustomized, outObjects...)
	claims, err := v.getClaims(run)
	if err != nil {
		return e("failed to get claims", err)
	}
	kustClaims := strategy.KustomizeClaims(claims)
	run.Spec.Objects.Claims = kustClaims
	return nil
}
