package model

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/libcompose/config"
	utils "github.com/docker/libcompose/utils"
	libYaml "github.com/docker/libcompose/yaml"
	"github.com/rancher/rancher-compose/preprocess"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
)

type mapLabel map[string]interface{}
type portArray []interface{}

//BindingProperty holds bindings
type BindingProperty map[string]interface{}

//ServiceBinding holds the fields for ServiceBinding
type ServiceBinding struct {
	Labels map[string]interface{} `json:"labels"`
	Ports  []interface{}          `json:"ports"`
}

//CreateBindingsRancher creates bindings property for rancher-compose.yml
func CreateBindingsRancher(pathToYml string) (BindingProperty, error) {
	var rawConfigRancher config.RawServiceMap
	var bindingsMap map[string]ServiceBinding
	var bindingPropertyMap BindingProperty
	var rawConfigDocker config.RawServiceMap
	var labels libYaml.SliceorMap

	bindingsMap = make(map[string]ServiceBinding)
	bindingPropertyMap = make(map[string]interface{})

	rancherFile := pathToYml + "/rancher-compose.yml"

	dockerFile := pathToYml + "/docker-compose.yml"
	_, err := os.Stat(dockerFile)
	if err == nil {
		yamlContent, err := ioutil.ReadFile(dockerFile)
		if err != nil {
			log.Errorf("Error in opening file : %v\n", err)
			return nil, err
		}
		err = yaml.Unmarshal(yamlContent, &rawConfigDocker)
		if err != nil {
			log.Errorf("Error during Unmarshal for file %s : %v\n", dockerFile, err)
			return nil, err
		}
		rawConfigDocker, err = preprocess.PreprocessServiceMap(rawConfigDocker)
		if err != nil {
			log.Errorf("Error during preprocess : %v\n", err)
			return nil, err
		}
	}

	rancherYaml, err := ioutil.ReadFile(rancherFile)
	if err != nil {
		log.Errorf("Error in opening file : %v\n", err)
		return nil, err
	}

	err = yaml.Unmarshal(rancherYaml, &rawConfigRancher)
	if err != nil {
		log.Errorf("Error during Unmarshal  for rancher-compose: %v\n", err)
		return nil, err
	}

	rawConfigRancher, err = preprocess.PreprocessServiceMap(rawConfigRancher)
	if err != nil {
		log.Errorf("Error in PreprocessServiceMap : %v\n", err)
		return nil, err
	}
	for key := range rawConfigRancher {
		if key != ".catalog" {
			newServiceBinding := ServiceBinding{}

			newServiceBinding.Labels = mapLabel{}
			newServiceBinding.Ports = portArray{}

			if rawConfigDocker[key]["labels"] != nil {
				err := utils.Convert(rawConfigDocker[key]["labels"], &labels)
				if err != nil {
					return nil, err
				}
				for k, v := range labels {
					newServiceBinding.Labels[k] = v
				}
			}
			if rawConfigDocker[key]["ports"] != nil {
				newServiceBinding.Ports = append(newServiceBinding.Ports, rawConfigDocker[key]["ports"])
			}
			bindingsMap[key] = newServiceBinding
		}
	}

	for key := range rawConfigDocker {
		if _, serviceParsed := bindingsMap[key]; serviceParsed {
			log.Debugf("Service bindings already provided")
			continue
		}
		if key != ".catalog" {
			newServiceBinding := ServiceBinding{}

			newServiceBinding.Labels = mapLabel{}
			newServiceBinding.Ports = portArray{}

			if rawConfigDocker[key]["labels"] != nil {
				err := utils.Convert(rawConfigDocker[key]["labels"], &labels)
				if err != nil {
					return nil, err
				}
				for k, v := range labels {
					newServiceBinding.Labels[k] = v
				}
			}
			if rawConfigDocker[key]["ports"] != nil {
				newServiceBinding.Ports = append(newServiceBinding.Ports, rawConfigDocker[key]["ports"])
			}
			bindingsMap[key] = newServiceBinding
		}
	}

	bindingPropertyMap["services"] = bindingsMap

	return bindingPropertyMap, nil
}
