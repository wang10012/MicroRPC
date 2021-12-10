package main

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
)

func test() {
	var wg sync.WaitGroup
	// pointer canSet
	typ := reflect.TypeOf(&wg)
	log.Println("typ.NumMethod:", typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		argv := make([]string, 0, method.Type.NumIn())
		returns := make([]string, 0, method.Type.NumOut())
		// j 从 1 开始，第 0 个入参是 wg 自己。
		for j := 1; j < method.Type.NumIn(); j++ {
			argv = append(argv, method.Type.In(j).Name())
		}
		for j := 0; j < method.Type.NumOut(); j++ {
			returns = append(returns, method.Type.Out(j).Name())
		}
		log.Printf("func (w *%s) %s(%s) %s",
			typ.Elem().Name(),
			method.Name,
			strings.Join(argv, ","),
			strings.Join(returns, ","))
	}
}

type User struct {
	name string
	age  int64
}

func main() {
	_map := new(map[string]int)
	m := reflect.TypeOf(_map)
	replyv := reflect.New(m.Elem())
	fmt.Println("now:", replyv)
	switch m.Elem().Kind() {
	case reflect.Map:
		fmt.Println("before,it is a map:", replyv)
		replyv.Elem().Set(reflect.MakeMap(m.Elem()))
		fmt.Println("after,it is a map:", replyv)
	case reflect.Slice:
		//replyv.Elem().Set(reflect.MakeSlice(m.Elem(), 0, 0))
		fmt.Println("it is a slice:", replyv)
	}
}
