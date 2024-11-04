package gp

/*
#include <Python.h>
*/
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

// pyObject is a wrapper type that holds a Python Object and automatically calls
// the Python Object's DecRef method during garbage collection.
type pyObject struct {
	obj *C.PyObject
}

func (obj *pyObject) Obj() *PyObject {
	if obj == nil {
		return nil
	}
	return obj.obj
}

func (obj *pyObject) Nil() bool {
	return obj == nil
}

func (obj *pyObject) Ensure() {
	if obj == nil {
		C.PyErr_Print()
		panic("nil Python object")
	}
}

// ----------------------------------------------------------------------------

type Object struct {
	*pyObject
}

func FromPy(obj *PyObject) Object {
	return newObject(obj)
}

func (obj Object) object() Object {
	return obj
}

func newObject(obj *PyObject) Object {
	if obj == nil {
		C.PyErr_Print()
		return Object{}
	}
	o := &pyObject{obj: obj}
	p := Object{o}
	runtime.SetFinalizer(o, func(o *pyObject) {
		// TODO: need better auto-release mechanism
		// C.Py_DecRef(o.obj)
	})
	return p
}

func (obj Object) Dir() List {
	return obj.Call("__dir__").AsList()
}

func (obj Object) Equals(other Objecter) bool {
	return C.PyObject_RichCompareBool(obj.obj, other.Obj(), C.Py_EQ) != 0
}

func (obj Object) Attr(name string) Object {
	cname := AllocCStr(name)
	o := C.PyObject_GetAttrString(obj.obj, cname)
	C.free(unsafe.Pointer(cname))
	return newObject(o)
}

func (obj Object) AttrFloat(name string) Float {
	return obj.Attr(name).AsFloat()
}

func (obj Object) AttrLong(name string) Long {
	return obj.Attr(name).AsLong()
}

func (obj Object) AttrString(name string) Str {
	return obj.Attr(name).AsStr()
}

func (obj Object) AttrBytes(name string) Bytes {
	return obj.Attr(name).AsBytes()
}

func (obj Object) AttrBool(name string) Bool {
	return obj.Attr(name).AsBool()
}

func (obj Object) AttrDict(name string) Dict {
	return obj.Attr(name).AsDict()
}

func (obj Object) AttrList(name string) List {
	return obj.Attr(name).AsList()
}

func (obj Object) AttrTuple(name string) Tuple {
	return obj.Attr(name).AsTuple()
}

func (obj Object) AttrFunc(name string) Func {
	return obj.Attr(name).AsFunc()
}

func (obj Object) SetAttr(name string, value any) {
	cname := AllocCStr(name)
	C.PyObject_SetAttrString(obj.obj, cname, From(value).obj)
	C.free(unsafe.Pointer(cname))
}

func (obj Object) IsLong() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyLong_Type) != 0
}

func (obj Object) IsFloat() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyFloat_Type) != 0
}

func (obj Object) IsComplex() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyComplex_Type) != 0
}

func (obj Object) IsStr() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyUnicode_Type) != 0
}

func (obj Object) IsBytes() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyBytes_Type) != 0
}

func (obj Object) IsBool() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyBool_Type) != 0
}

func (obj Object) IsList() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyList_Type) != 0
}

func (obj Object) IsTuple() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyTuple_Type) != 0
}

func (obj Object) IsDict() bool {
	return C.Py_IS_TYPE(obj.obj, &C.PyDict_Type) != 0
}

func (obj Object) AsFloat() Float {
	return Cast[Float](obj)
}

func (obj Object) AsLong() Long {
	return Cast[Long](obj)
}

func (obj Object) AsComplex() Complex {
	return Cast[Complex](obj)
}

func (obj Object) AsStr() Str {
	return Cast[Str](obj)
}

func (obj Object) AsBytes() Bytes {
	return Cast[Bytes](obj)
}

func (obj Object) AsBool() Bool {
	return Cast[Bool](obj)
}

func (obj Object) AsDict() Dict {
	return Cast[Dict](obj)
}

func (obj Object) AsList() List {
	return Cast[List](obj)
}

func (obj Object) AsTuple() Tuple {
	return Cast[Tuple](obj)
}

func (obj Object) AsFunc() Func {
	return Cast[Func](obj)
}

func (obj Object) AsModule() Module {
	return Cast[Module](obj)
}

func (obj Object) Call(name string, args ...any) Object {
	fn := Cast[Func](obj.Attr(name))
	argsTuple, kwArgs := splitArgs(args...)
	if kwArgs == nil {
		return fn.CallObject(argsTuple)
	} else {
		return fn.CallObjectKw(argsTuple, kwArgs)
	}
}

func (obj Object) Repr() string {
	return newStr(C.PyObject_Repr(obj.obj)).String()
}

func (obj Object) Type() Object {
	return newObject(C.PyObject_Type(obj.Obj()))
}

func (obj Object) String() string {
	return newStr(C.PyObject_Str(obj.obj)).String()
}

func (obj Object) Obj() *PyObject {
	if obj.Nil() {
		return nil
	}
	return obj.pyObject.obj
}

func From(v any) Object {
	switch v := v.(type) {
	case Objecter:
		return newObject(v.Obj())
	case int8:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case int16:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case int32:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case int64:
		return newObject(C.PyLong_FromLongLong(C.longlong(v)))
	case int:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case uint8:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case uint16:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case uint32:
		return newObject(C.PyLong_FromLong(C.long(v)))
	case uint64:
		return newObject(C.PyLong_FromUnsignedLongLong(C.ulonglong(v)))
	case uint:
		return newObject(C.PyLong_FromUnsignedLong(C.ulong(v)))
	case float64:
		return newObject(C.PyFloat_FromDouble(C.double(v)))
	case string:
		cstr := AllocCStr(v)
		o := C.PyUnicode_FromString(cstr)
		C.free(unsafe.Pointer(cstr))
		return newObject(o)
	case complex128:
		return MakeComplex(v).Object
	case complex64:
		return MakeComplex(complex128(v)).Object
	case []byte:
		return MakeBytes(v).Object
	case bool:
		if v {
			return True().Object
		} else {
			return False().Object
		}
	case *C.PyObject:
		return newObject(v)
	default:
		vv := reflect.ValueOf(v)
		switch vv.Kind() {
		case reflect.Ptr:
			return From(vv.Elem().Interface())
		case reflect.Slice:
			return fromSlice(vv).Object
		case reflect.Map:
			return fromMap(vv).Object
		case reflect.Struct:
			return fromStruct(vv)
		}
		panic(fmt.Errorf("unsupported type for Python: %T\n", v))
	}
}

func ToValue(obj Object, v reflect.Value) bool {
	// Handle nil pointer
	if !v.IsValid() || !v.CanSet() {
		return false
	}

	switch v.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		if obj.IsLong() {
			v.SetInt(Cast[Long](obj).Int64())
		} else {
			return false
		}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		if obj.IsLong() {
			v.SetUint(Cast[Long](obj).Uint64())
		} else {
			return false
		}
	case reflect.Float32, reflect.Float64:
		if obj.IsFloat() || obj.IsLong() {
			v.SetFloat(Cast[Float](obj).Float64())
		} else {
			return false
		}
	case reflect.Complex64, reflect.Complex128:
		if obj.IsComplex() {
			v.SetComplex(Cast[Complex](obj).Complex128())
		} else {
			return false
		}
	case reflect.String:
		if obj.IsStr() {
			v.SetString(Cast[Str](obj).String())
		} else {
			return false
		}
	case reflect.Bool:
		if obj.IsBool() {
			v.SetBool(Cast[Bool](obj).Bool())
		} else {
			return false
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 { // []byte
			if obj.IsBytes() {
				v.SetBytes(Cast[Bytes](obj).Bytes())
			} else {
				return false
			}
		} else {
			if obj.IsList() {
				list := Cast[List](obj)
				l := list.Len()
				slice := reflect.MakeSlice(v.Type(), l, l)
				for i := 0; i < l; i++ {
					item := list.GetItem(i)
					ToValue(item, slice.Index(i))
				}
				v.Set(slice)
			} else {
				return false
			}
		}
	case reflect.Map:
		if obj.IsDict() {
			t := v.Type()
			v.Set(reflect.MakeMap(t))
			dict := Cast[Dict](obj)
			dict.ForEach(func(key, value Object) {
				vk := reflect.New(t.Key()).Elem()
				vv := reflect.New(t.Elem()).Elem()
				if !ToValue(key, vk) || !ToValue(value, vv) {
					panic(fmt.Errorf("failed to convert key or value to %v", t.Key()))
				}
				v.SetMapIndex(vk, vv)
			})
		} else {
			return false
		}
	case reflect.Struct:
		if obj.IsDict() {
			dict := Cast[Dict](obj)
			t := v.Type()
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				key := goNameToPythonName(field.Name)
				value := dict.Get(MakeStr(key))
				if !ToValue(value, v.Field(i)) {
					panic(fmt.Errorf("failed to convert value to %v", field.Name))
				}
			}
		} else {
			tyMeta := typeMetaMap[obj.Type().Obj()]
			if tyMeta == nil {
				return false
			}
			wrapper := (*wrapperType)(unsafe.Pointer(obj.Obj()))
			vPtr := unsafe.Pointer(&wrapper.v)
			v.Set(reflect.NewAt(tyMeta.typ, vPtr).Elem())
			return true
		}
	default:
		panic(fmt.Errorf("unsupported type conversion from Python object to %v", v.Type()))
	}
	return true
}

func fromSlice(v reflect.Value) List {
	l := v.Len()
	list := newList(C.PyList_New(C.Py_ssize_t(l)))
	for i := 0; i < l; i++ {
		list.SetItem(i, From(v.Index(i).Interface()))
	}
	return list
}

func fromMap(v reflect.Value) Dict {
	dict := newDict(C.PyDict_New())
	iter := v.MapRange()
	for iter.Next() {
		dict.Set(From(iter.Key().Interface()), From(iter.Value().Interface()))
	}
	return dict
}

func fromStruct(v reflect.Value) Object {
	ty := v.Type()
	if typeObj, ok := pyTypeMap[ty]; ok {
		obj := newObject(C._PyObject_New((*C.PyTypeObject)(unsafe.Pointer(typeObj))))
		for i := 0; i < ty.NumField(); i++ {
			field := ty.Field(i)
			key := goNameToPythonName(field.Name)
			obj.SetAttr(key, From(v.Field(i).Interface()))
		}
		return obj
	}
	dict := newDict(C.PyDict_New())
	for i := 0; i < ty.NumField(); i++ {
		field := ty.Field(i)
		key := goNameToPythonName(field.Name)
		dict.Set(MakeStr(key).Object, From(v.Field(i).Interface()))
	}
	return dict.Object
}
