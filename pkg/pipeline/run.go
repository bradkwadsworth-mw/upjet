/*
 Copyright 2021 The Crossplane Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crossplane-contrib/terrajet/pkg/config"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
)

func Run(pc config.Provider) { // nolint:gocyclo
	// Cyclomatic complexity of this function is above our goal of 10,
	// and it establishes a Terrajet code generation pipeline that's very similar
	// to other Terrajet based providers.
	// delete API dirs

	//genConfig.SetResourceConfigurations()
	wd, err := os.Getwd()
	if err != nil {
		panic(errors.Wrap(err, "cannot get working directory"))
	}
	fmt.Println(wd)
	groupVersions := map[string]map[string]map[string]*config.Resource{}
	for name, resource := range pc.Resources {
		fmt.Printf("Generating code for resource: %s\n", name)

		if len(groupVersions[resource.Group]) == 0 {
			groupVersions[resource.Group] = map[string]map[string]*config.Resource{}
		}
		if len(groupVersions[resource.Group][resource.Version]) == 0 {
			groupVersions[resource.Group][resource.Version] = map[string]*config.Resource{}
		}
		groupVersions[resource.Group][resource.Version][name] = resource
	}

	count := 0
	versionPkgList := []string{
		// TODO(turkenh): a parameter for v1alpha1 here?
		filepath.Join(pc.ModulePath, "apis", "v1alpha1"),
	}
	controllerPkgList := []string{
		filepath.Join(pc.ModulePath, "internal", "controller", "providerconfig"),
	}
	for group, versions := range groupVersions {
		for version, resources := range versions {
			versionGen := NewVersionGenerator(wd, pc.ModulePath, strings.ToLower(group)+pc.GroupSuffix, version)

			crdGen := NewCRDGenerator(versionGen.Package(), versionGen.DirectoryPath(), strings.ToLower(group)+pc.GroupSuffix, pc.ShortName)
			tfGen := NewTerraformedGenerator(versionGen.Package(), versionGen.DirectoryPath())
			ctrlGen := NewControllerGenerator(wd, pc.ModulePath, strings.ToLower(group)+pc.GroupSuffix)

			keys := make([]string, len(resources))
			i := 0
			for k := range resources {
				keys[i] = k
				i++
			}
			sort.Strings(keys)

			for _, name := range keys {

				resourceConfig := resources[name]

				if err := crdGen.Generate(resourceConfig); err != nil {
					panic(errors.Wrap(err, "cannot generate crd"))
				}
				if err := tfGen.Generate(resourceConfig); err != nil {
					panic(errors.Wrap(err, "cannot generate terraformed"))
				}
				ctrlPkgPath, err := ctrlGen.Generate(resourceConfig, versionGen.Package().Path())
				if err != nil {
					panic(errors.Wrap(err, "cannot generate controller"))
				}
				controllerPkgList = append(controllerPkgList, ctrlPkgPath)
				count++
			}

			if err := versionGen.Generate(); err != nil {
				panic(errors.Wrap(err, "cannot generate version files"))
			}
			versionPkgList = append(versionPkgList, versionGen.Package().Path())
		}
	}

	if err := NewRegisterGenerator(wd, pc.ModulePath).Generate(versionPkgList); err != nil {
		panic(errors.Wrap(err, "cannot generate register file"))
	}
	if err := NewSetupGenerator(wd, pc.ModulePath).Generate(controllerPkgList); err != nil {
		panic(errors.Wrap(err, "cannot generate setup file"))
	}
	if out, err := exec.Command("bash", "-c", "goimports -w $(find apis -iname 'zz_*')").CombinedOutput(); err != nil {
		panic(errors.Wrap(err, "cannot run goimports for apis folder: "+string(out)))
	}
	if out, err := exec.Command("bash", "-c", "goimports -w $(find internal -iname 'zz_*')").CombinedOutput(); err != nil {
		panic(errors.Wrap(err, "cannot run goimports for internal folder: "+string(out)))
	}
	fmt.Printf("\nGenerated %d resources!\n", count)
}
