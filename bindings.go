package main

import (
	"fmt"
	config "github.com/docker/libcompose/config"
	yaml "github.com/cloudfoundry-incubator/candiedyaml"
	"io/ioutil"
	log "github.com/Sirupsen/logrus"
)

func CreateBindings() {
	var servConf []config.ServiceConfigV1
	yamlContent, err := ioutil.ReadFile("rancher-compose.yml")
	if err != nil {
		log.Errorf("Error in opening file : %v\n",err)
	} else {
		err = yaml.Unmarshal(yamlContent, &servConf)
		if err != nil {
			log.Errorf("Error during Unmarshal : %v\n",err)
		} else {
			fmt.Printf("Unmarshaled : %v\n",servConf)
		}
	}
}


func preprocessServiceMap(serviceMap RawServiceMap) RawServiceMap {
	newServiceMap := make(RawServiceMap)

	for k, v := range serviceMap {
		newServiceMap[k] = make(RawService)

		for k2, v2 := range v {
			if k2 == "environment" || k2 == "labels" {
				newServiceMap[k][k2] = preprocess(v2, true)
			} else {
				newServiceMap[k][k2] = preprocess(v2, false)
			}

		}
	}

	return newServiceMap
}

func preprocess(item interface{}, replaceTypes bool) interface{} {
        if item == nil {
            return nil
        }

	switch typedDatas := item.(type) {

	case map[interface{}]interface{}:
		newMap := make(map[interface{}]interface{})

		for key, value := range typedDatas {
			newMap[key] = preprocess(value, replaceTypes)
		}
		return newMap

	case []interface{}:
		// newArray := make([]interface{}, 0) will cause golint to complain
		var newArray []interface{}
		newArray = make([]interface{}, 0)

		for _, value := range typedDatas {
			newArray = append(newArray, preprocess(value, replaceTypes))
		}
		return newArray
	default:
		if replaceTypes {
			return fmt.Sprint(item)
		}
		return item
	}
}