/*
Copyright 2014 The Kubernetes Authors.

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

package apply

import (
	"encoding/json"
	"reflect"

	"github.com/goph/emperror"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

const LastAppliedConfig = "banzaicloud.com/last-applied"

var metadataAccessor = meta.NewAccessor()

// GetOriginalConfiguration retrieves the original configuration of the object
// from the annotation, or nil if no annotation was found.
func GetOriginalConfiguration(obj runtime.Object) ([]byte, error) {
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, err
	}

	if annots == nil {
		return nil, nil
	}

	original, ok := annots[LastAppliedConfig]
	if !ok {
		return nil, nil
	}

	return []byte(original), nil
}

// SetOriginalConfiguration sets the original configuration of the object
// as the annotation on the object for later use in computing a three way patch.
func SetOriginalConfiguration(obj runtime.Object, original []byte) error {
	if len(original) < 1 {
		return nil
	}

	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return err
	}

	if annots == nil {
		annots = map[string]string{}
	}

	annots[LastAppliedConfig] = string(original)
	return metadataAccessor.SetAnnotations(obj, annots)
}

// GetModifiedConfiguration retrieves the modified configuration of the object.
// If annotate is true, it embeds the result as an annotation in the modified
// configuration. If an object was read from the command input, it will use that
// version of the object. Otherwise, it will use the version from the server.
func GetModifiedConfiguration(obj runtime.Object, annotate bool) ([]byte, error) {
	// First serialize the object without the annotation to prevent recursion,
	// then add that serialization to it as the annotation and serialize it again.
	var modified []byte

	// Otherwise, use the server side version of the object.
	// Get the current annotations from the object.
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, err
	}

	if annots == nil {
		annots = map[string]string{}
	}

	original := annots[LastAppliedConfig]
	delete(annots, LastAppliedConfig)
	if err := metadataAccessor.SetAnnotations(obj, annots); err != nil {
		return nil, err
	}

	modified, err = json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	if annotate {
		annots[LastAppliedConfig] = string(modified)
		if err := metadataAccessor.SetAnnotations(obj, annots); err != nil {
			return nil, err
		}

		modified, err = json.Marshal(obj)
		if err != nil {
			return nil, err
		}
	}

	// Restore the object to its original condition.
	annots[LastAppliedConfig] = original
	if err := metadataAccessor.SetAnnotations(obj, annots); err != nil {
		return nil, err
	}

	return modified, nil
}

// UpdateApplyAnnotation calls CreateApplyAnnotation if the last applied
// configuration annotation is already present. Otherwise, it does nothing.
func UpdateApplyAnnotation(obj runtime.Object) error {
	if original, err := GetOriginalConfiguration(obj); err != nil || len(original) <= 0 {
		return err
	}
	return CreateApplyAnnotation(obj)
}

// CreateApplyAnnotation gets the modified configuration of the object,
// without embedding it again, and then sets it on the object as the annotation.
func CreateApplyAnnotation(obj runtime.Object) error {
	modified, err := GetModifiedConfiguration(obj, false)
	if err != nil {
		return err
	}
	modifiedWithoutNulls, _, err := DeleteNullInJson(modified)
	if err != nil {
		return err
	}
	return SetOriginalConfiguration(obj, modifiedWithoutNulls)
}

// CreateOrUpdateAnnotation creates the annotation used by
// kubectl apply only when createAnnotation is true
// Otherwise, only update the annotation when it already exists
func CreateOrUpdateAnnotation(createAnnotation bool, obj runtime.Object) error {
	if createAnnotation {
		return CreateApplyAnnotation(obj)
	}
	return UpdateApplyAnnotation(obj)
}


func DeleteNullInJson(patch []byte) ([]byte, map[string]interface{}, error) {
	var patchMap map[string]interface{}

	err := json.Unmarshal(patch, &patchMap)
	if err != nil {
		return nil, nil, emperror.Wrap(err, "could not unmarshal json patch")
	}

	filteredMap, err := deleteNullInObj(patchMap)
	if err != nil {
		return nil, nil, emperror.Wrap(err, "could not delete null values from patch map")
	}

	o, err := json.Marshal(filteredMap)
	if err != nil {
		return nil, nil, emperror.Wrap(err, "could not marshal filtered patch map")
	}

	return o, filteredMap, err
}

func deleteNullInObj(m map[string]interface{}) (map[string]interface{}, error) {
	var err error
	filteredMap := make(map[string]interface{})

	for key, val := range m {
		if val == nil {
			continue
		}

		switch typedVal := val.(type) {
		default:
			return nil, errors.Errorf("unknown type: %v", reflect.TypeOf(typedVal))
		case []interface{}:
			slice, err := deleteNullInSlice(typedVal)
			if err != nil {
				return nil, errors.Errorf("could not delete null values from subslice")
			}
			filteredMap[key] = slice
		case string, float64, bool, int64, nil:
			filteredMap[key] = val
		case map[string]interface{}:
			if len(typedVal) == 0 {
				filteredMap[key] = typedVal
				continue
			}

			var filteredSubMap map[string]interface{}
			filteredSubMap, err = deleteNullInObj(typedVal)
			if err != nil {
				return nil, emperror.Wrap(err, "could not delete null values from filtered sub map")
			}

			if len(filteredSubMap) != 0 {
				filteredMap[key] = filteredSubMap
			}
		}
	}
	return filteredMap, nil
}

func deleteNullInSlice(m []interface{}) ([]interface{}, error) {
	filteredSlice := make([]interface{}, len(m))
	for key, val := range m {
		if val == nil {
			continue
		}
		switch typedVal := val.(type) {
		default:
			return nil, errors.Errorf("unknown type: %v", reflect.TypeOf(typedVal))
		case []interface{}:
			filteredSubSlice, err := deleteNullInSlice(typedVal)
			if err != nil {
				return nil, errors.Errorf("could not delete null values from subslice")
			}
			filteredSlice[key] = filteredSubSlice
		case string, float64, bool, int64, nil:
			filteredSlice[key] = val
		case map[string]interface{}:
			filteredMap, err := deleteNullInObj(typedVal)
			if err != nil {
				return nil, emperror.Wrap(err, "could not delete null values from filtered sub map")
			}
			filteredSlice[key] = filteredMap
		}
	}
	return filteredSlice, nil
}
