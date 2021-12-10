package MicroRPC

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

type method struct {
	_method   reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	numCalled uint64
}

func (m *method) NumCalled() uint64 {
	// atomic lock
	return atomic.LoadUint64(&m.numCalled)
}

func (m *method) newArgv() reflect.Value {
	var argv reflect.Value
	// pointer type
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
		// value type
	} else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

func (m *method) newReplyv() reflect.Value {
	// reply must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	// Todo:why set?
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

type service struct {
	name     string
	_type    reflect.Type
	instance reflect.Value
	methods  map[string]*method // key: method.Name
}

// newService :make sure instance a pointer to set value
func newService(instance interface{}) *service {
	s := new(service)
	s.instance = reflect.ValueOf(instance)
	// get value of a pointer,use Indirect().Type()
	// To get name, get value of the pointer
	s.name = reflect.Indirect(s.instance).Type().Name()
	s._type = reflect.TypeOf(instance)
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethods()
	return s
}

func (s *service) registerMethods() {
	s.methods = make(map[string]*method)
	for i := 0; i < s._type.NumMethod(); i++ {
		m := s._type.Method(i)
		mType := m.Type
		// reflect argv including instance
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		// Todo:why *error.Elem() instead of error
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.methods[m.Name] = &method{
			_method:   m,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, m.Name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *service) call(m *method, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalled, 1)
	f := m._method.Func
	returnValues := f.Call([]reflect.Value{s.instance, argv, replyv})
	if errInterface := returnValues[0].Interface(); errInterface != nil {
		return errInterface.(error)
	}
	return nil
}
