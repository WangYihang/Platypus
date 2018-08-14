package reflection

import (
	"reflect"
)

func Invoke(any interface{}, name string, args ...interface{}) {
	params := make([]reflect.Value, len(args))
	for i, _ := range args {
		params[i] = reflect.ValueOf(args[i])
	}
	reflect.ValueOf(any).MethodByName(name).Call(params)
}

func GetAllMethods(any interface{}) []string {
	var methods []string
	anyType := reflect.TypeOf(any)
	for i := 0; i < anyType.NumMethod(); i++ {
		method := anyType.Method(i)
		methods = append(methods, method.Name)
	}
	return methods
}

func Contains(target interface{}, obj interface{}) bool {
	targetValue := reflect.ValueOf(target)
	switch reflect.TypeOf(target).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true
			}
		}
	case reflect.Map:
		if targetValue.MapIndex(reflect.ValueOf(obj)).IsValid() {
			return true
		}
	}
	return false
}
