package model

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	yaml "github.com/cloudfoundry-incubator/candiedyaml"
	"github.com/docker/libcompose/config"
	"github.com/docker/libcompose/utils"
	"github.com/rancher/rancher-compose/preprocess"
	"github.com/rancher/rancher-compose/rancher"
	"io/ioutil"
	"os"
	"reflect"
)

type mapLabel map[string]string

//BindingProperty holds bindings
type BindingProperty map[string]interface{}

//ServiceBinding holds the fields for ServiceBinding
type ServiceBinding struct {
	Scale  int               `json:"scale"`
	Labels map[string]string `json:"labels"`
	Ports  []string          `json:"ports"`
}

//CreateBindingsRancher creates bindings property for rancher-compose.yml
func CreateBindingsRancher(pathToYml string) BindingProperty {
	var rawConfigRancher config.RawServiceMap
	var rancherComposeMap map[string]rancher.RancherConfig
	var bindingsMap map[string]ServiceBinding
	// BindingProperty = make(map[string]interface{})
	var bindingPropertyMap BindingProperty

	var configMap map[string]config.ServiceConfigV1
	var rawConfigDocker config.RawServiceMap

	bindingsMap = make(map[string]ServiceBinding)
	bindingPropertyMap = make(map[string]interface{})

	rancherFile := pathToYml + "/rancher-compose.yml"

	dockerFile := pathToYml + "/docker-compose.yml"
	_, err := os.Stat(dockerFile)
	if err == nil {
		yamlContent, err := ioutil.ReadFile(dockerFile)
		if err != nil {
			log.Errorf("Error in opening file : %v\n", err)
		} else {
			err = yaml.Unmarshal(yamlContent, &rawConfigDocker)
			if err != nil {
				log.Errorf("Error during Unmarshal for file %s : %v\n", dockerFile, err)
			} else {
				rawConfigDocker, err := preprocess.PreprocessServiceMap(rawConfigDocker)
				utils.Convert(rawConfigDocker, &configMap)
				if err != nil {
					log.Errorf("Error")
				}
			}
		}
	}

	rancherYaml, err := ioutil.ReadFile(rancherFile)
	if err != nil {
		log.Errorf("Error in opening file : %v\n", err)
		return nil
	}

	err = yaml.Unmarshal(rancherYaml, &rawConfigRancher)
	if err != nil {
		log.Errorf("Error during Unmarshal  for rancher-compose: %v\n", err)
	} else {
		rawConfigRancher, err := preprocess.PreprocessServiceMap(rawConfigRancher)
		utils.Convert(rawConfigRancher, &rancherComposeMap)
		if err != nil {
			fmt.Printf("Error")
		} else {
			keys := reflect.ValueOf(rancherComposeMap).MapKeys()

			for _, key := range keys {
				if key.String() != ".catalog" {
					newServiceBinding := ServiceBinding{}

					newServiceBinding.Labels = mapLabel{}
					newServiceBinding.Ports = []string{}

					newServiceBinding.Scale = rancherComposeMap[key.String()].Scale
					if configMap[key.String()].Labels != nil {
						newServiceBinding.Labels = configMap[key.String()].Labels
					}
					if configMap[key.String()].Ports != nil {
						newServiceBinding.Ports = configMap[key.String()].Ports
					}
					bindingsMap[key.String()] = newServiceBinding
				}
			}

			keys = reflect.ValueOf(configMap).MapKeys()
			for _, key := range keys {
				if _, serviceParsed := bindingsMap[key.String()]; serviceParsed {
					log.Debugf("Service bindings already provided")
					continue
				}
				if key.String() != ".catalog" {
					newServiceBinding := ServiceBinding{}

					newServiceBinding.Labels = mapLabel{}
					newServiceBinding.Ports = []string{}

					if configMap[key.String()].Labels != nil {
						newServiceBinding.Labels = configMap[key.String()].Labels
					}
					if configMap[key.String()].Ports != nil {
						newServiceBinding.Ports = configMap[key.String()].Ports
					}
					bindingsMap[key.String()] = newServiceBinding
				}
			}

			bindingPropertyMap["services"] = bindingsMap

			return bindingPropertyMap
		}
	}
	return nil
}
