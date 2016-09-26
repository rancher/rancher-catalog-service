package model

import (
	utils "github.com/docker/libcompose/utils"
	libYaml "github.com/docker/libcompose/yaml"
	"reflect"
	"testing"
)

func TestCreateBindings(t *testing.T) {
	var labelsCompose libYaml.SliceorMap
	var labels libYaml.SliceorMap
	var ports PortArray

	bindingPropertyMap, err := ExtractBindings([]byte(`
test_v1:
  ports:
  - 9000:9000/tcp
  labels:
    label_1: value_1
  tty: true
  image: foo`))
	if err != nil {
		t.Fatal(err)
	}

	labelsCompose = make(map[string]string)
	labelsCompose["label_1"] = "value_1"
	portsCompose := PortArray{}
	portsCompose = append(portsCompose, "9000:9000/tcp")

	if len(bindingPropertyMap) != 1 {
		t.Fatal("Bindings not created")
	}

	if _, ok := bindingPropertyMap["services"]; ok {
		service := bindingPropertyMap["services"].(map[string]ServiceBinding)

		err = utils.Convert(service["test_v1"].Labels, &labels)
		if err != nil {
			t.Fatal(err)
		}

		if !(reflect.DeepEqual(labels, labelsCompose)) {
			t.Fatal("Bindings labels incorrect")
		}

		err = utils.Convert(service["test_v1"].Ports, &ports)
		if err != nil {
			t.Fatal(err)
		}

		if !(reflect.DeepEqual(ports, portsCompose)) {
			t.Fatal("Bindings ports incorrect")
		}
	}

	bindingPropertyMap, err = ExtractBindings([]byte(`
version: '2'
services:
  test_v2:
    labels:
      label_2: value_2
    ports:
    - 9001:9001/tcp
`))
	if err != nil {
		t.Fatal(err)
	}

	labelsCompose = make(map[string]string)
	labelsCompose["label_2"] = "value_2"
	portsCompose = PortArray{}
	portsCompose = append(portsCompose, "9001:9001/tcp")

	if len(bindingPropertyMap) != 1 {
		t.Fatal("Bindings not created")
	}

	if _, ok := bindingPropertyMap["services"]; ok {
		service := bindingPropertyMap["services"].(map[string]ServiceBinding)

		err = utils.Convert(service["test_v2"].Labels, &labels)
		if err != nil {
			t.Fatal(err)
		}

		if !(reflect.DeepEqual(labels, labelsCompose)) {
			t.Fatal("Bindings labels incorrect")
		}

		err = utils.Convert(service["test_v2"].Ports, &ports)
		if err != nil {
			t.Fatal(err)
		}

		if !(reflect.DeepEqual(ports, portsCompose)) {
			t.Fatal("Bindings ports incorrect")
		}
	}
}
