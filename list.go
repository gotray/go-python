package gp

/*
#include <Python.h>
*/
import "C"
import "fmt"

type List struct {
	Object
}

func newList(obj *PyObject) List {
	return List{newObject(obj)}
}

func MakeList(args ...any) List {
	list := newList(C.PyList_New(C.Py_ssize_t(len(args))))
	for i, arg := range args {
		obj := From(arg)
		list.SetItem(i, obj)
	}
	return list
}

func (l List) GetItem(index int) Object {
	v := C.PyList_GetItem(l.obj, C.Py_ssize_t(index))
	C.Py_IncRef(v)
	return newObject(v)
}

func (l List) SetItem(index int, item Objecter) {
	itemObj := item.Obj()
	C.Py_IncRef(itemObj)
	r := C.PyList_SetItem(l.obj, C.Py_ssize_t(index), itemObj)
	check(r == 0, fmt.Sprintf("failed to set item %d in list", index))
}

func (l List) Len() int {
	return int(C.PyList_Size(l.obj))
}

func (l List) Append(obj Objecter) {
	r := C.PyList_Append(l.obj, obj.Obj())
	check(r == 0, "failed to append item to list")
}
