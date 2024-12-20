package gp

/*
#include <Python.h>
*/
import "C"
import "unsafe"

type Module struct {
	Object
}

func newModule(obj *cPyObject) Module {
	return Module{newObject(obj)}
}

func ImportModule(name string) Module {
	cname := AllocCStr(name)
	mod := C.PyImport_ImportModule(cname)
	C.free(unsafe.Pointer(cname))
	return newModule(mod)
}

func GetModule(name string) Module {
	return newModule(C.PyImport_GetModule(MakeStr(name).obj))
}

func (m Module) Dict() Dict {
	return newDict(C.PyModule_GetDict(m.obj))
}

func (m Module) AddObject(name string, obj Object) int {
	cname := AllocCStr(name)
	r := int(C.PyModule_AddObjectRef(m.obj, cname, obj.obj))
	C.free(unsafe.Pointer(cname))
	return r
}

func (m Module) Name() string {
	return C.GoString(C.PyModule_GetName(m.obj))
}

func CreateModule(name string) Module {
	mod := C.PyModule_New(AllocCStrDontFree(name))
	return newModule(mod)
}

func GetModuleDict() Dict {
	return newDict(C.PyImport_GetModuleDict())
}
