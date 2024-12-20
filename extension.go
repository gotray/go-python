package gp

/*
#include <Python.h>
#include <structmember.h>
#include <moduleobject.h>

#include "wrap.h"
extern PyObject* wrapperAlloc(PyTypeObject* type, Py_ssize_t size);
extern void wrapperDealloc(PyObject* self);
extern int wrapperInit(PyObject* self, PyObject* args);
static int isModule(PyObject* ob)
{
	return PyObject_TypeCheck(ob, &PyModule_Type);
}
*/
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"unicode"
	"unsafe"
)

func FuncOf(fn any) Func {
	return CreateFunc("", fn, "")
}

func CreateFunc(name string, fn any, doc string) Func {
	m := MainModule()
	return m.AddMethod(name, fn, doc)
}

type wrapperType struct {
	cPyObject
	goObj  any
	holder *objectHolder
}

type objectHolder struct {
	obj  any
	prev *objectHolder
	next *objectHolder
}

type slotMeta struct {
	name       string
	methodName string
	fn         any
	doc        string
	hasRecv    bool         // whether it has a receiver
	index      int          // used for member type
	typ        reflect.Type // member/method type
	def        *C.PyMethodDef
}

type typeMeta struct {
	typ     reflect.Type
	init    *slotMeta
	methods map[uint]*slotMeta
}

func allocWrapper(typ *C.PyTypeObject, obj any) *wrapperType {
	self := C.PyType_GenericAlloc(typ, 0)
	check(self != nil, "failed to allocate wrapper")
	wrapper := (*wrapperType)(unsafe.Pointer(self))
	holder := new(objectHolder)
	holder.obj = obj
	maps := getGlobalData()
	maps.holders.PushFront(holder)
	wrapper.goObj = holder.obj
	wrapper.holder = holder
	return wrapper
}

func freeWrapper(wrapper *wrapperType) {
	maps := getGlobalData()
	maps.holders.Remove(wrapper.holder)
}

//export wrapperAlloc
func wrapperAlloc(typ *C.PyTypeObject, size C.Py_ssize_t) *C.PyObject {
	maps := getGlobalData()
	meta := maps.typeMetas[(*C.PyObject)(unsafe.Pointer(typ))]
	wrapper := allocWrapper(typ, reflect.New(meta.typ).Interface())
	check(wrapper != nil, "failed to allocate wrapper")
	return (*C.PyObject)(unsafe.Pointer(wrapper))
}

//export wrapperDealloc
func wrapperDealloc(self *C.PyObject) {
	wrapper := (*wrapperType)(unsafe.Pointer(self))
	freeWrapper(wrapper)
	C.PyObject_Free(unsafe.Pointer(self))
}

//export wrapperInit
func wrapperInit(self, args *C.PyObject) C.int {
	typ := (*C.PyObject)(self).ob_type
	maps := getGlobalData()
	typeMeta := maps.typeMetas[(*C.PyObject)(unsafe.Pointer(typ))]
	check(typeMeta != nil, "type not registered")
	check(typeMeta.init != nil, "init method not found")
	if wrapperMethod_(typeMeta, typeMeta.init, self, args, 0) == nil {
		return -1
	}
	return 0
}

//export getterMethod
func getterMethod(self *C.PyObject, _closure unsafe.Pointer, methodId C.int) *C.PyObject {
	maps := getGlobalData()
	typeMeta := maps.typeMetas[(*C.PyObject)(unsafe.Pointer(self.ob_type))]
	check(typeMeta != nil, fmt.Sprintf("type %v not registered", FromPy(self)))
	methodMeta := typeMeta.methods[uint(methodId)]
	check(methodMeta != nil, fmt.Sprintf("getter method %d not found", methodId))

	wrapper := (*wrapperType)(unsafe.Pointer(self))
	goPtr := reflect.ValueOf(wrapper.goObj)
	goValue := goPtr.Elem()
	field := goValue.Field(methodMeta.index)

	fieldType := field.Type()
	if fieldType.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct {
		if field.IsNil() {
			return C.Py_None
		}
		if pyType, ok := maps.pyTypes[fieldType.Elem()]; ok {
			newWrapper := allocWrapper((*C.PyTypeObject)(unsafe.Pointer(pyType)), field.Interface())
			check(newWrapper != nil, "failed to allocate wrapper for nested struct pointer")
			return (*C.PyObject)(unsafe.Pointer(newWrapper))
		}
	} else if field.Kind() == reflect.Struct {
		if pyType, ok := maps.pyTypes[field.Type()]; ok {
			baseAddr := goPtr.UnsafePointer()
			fieldAddr := unsafe.Add(baseAddr, typeMeta.typ.Field(methodMeta.index).Offset)
			fieldPtr := reflect.NewAt(fieldType, fieldAddr).Interface()
			newWrapper := allocWrapper((*C.PyTypeObject)(unsafe.Pointer(pyType)), fieldPtr)
			check(newWrapper != nil, "failed to allocate wrapper for nested struct")
			return (*C.PyObject)(unsafe.Pointer(newWrapper))
		}
	}
	return From(field.Interface()).cpyObj()
}

//export setterMethod
func setterMethod(self, value *C.PyObject, _closure unsafe.Pointer, methodId C.int) C.int {
	maps := getGlobalData()
	typeMeta := maps.typeMetas[(*C.PyObject)(unsafe.Pointer(self.ob_type))]
	check(typeMeta != nil, fmt.Sprintf("type %v not registered", FromPy(self)))
	methodMeta := typeMeta.methods[uint(methodId)]
	check(methodMeta != nil, fmt.Sprintf("setter method %d not found", methodId))

	wrapper := (*wrapperType)(unsafe.Pointer(self))
	goPtr := reflect.ValueOf(wrapper.goObj)
	goValue := goPtr.Elem()

	structValue := goValue
	if !structValue.CanSet() {
		SetTypeError(fmt.Errorf("struct value cannot be set"))
		return -1
	}

	field := structValue.Field(methodMeta.index)
	if !field.CanSet() {
		SetTypeError(fmt.Errorf("field %s cannot be set", methodMeta.name))
		return -1
	}

	fieldType := field.Type()
	if fieldType.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct {
		if C.Py_Is(value, C.Py_None) != 0 {
			field.Set(reflect.Zero(fieldType))
			return 0
		}
		if C.Py_IS_TYPE(value, &C.PyDict_Type) != 0 {
			if field.IsNil() {
				field.Set(reflect.New(fieldType.Elem()))
			}
			if !ToValue(FromPy(value), field.Elem()) {
				SetTypeError(fmt.Errorf("failed to convert dict to %s", fieldType.Elem()))
				return -1
			}
		} else {
			pyType := C.Py_TYPE(value)
			if _, ok := maps.typeMetas[(*C.PyObject)(unsafe.Pointer(pyType))]; !ok {
				SetTypeError(fmt.Errorf("invalid value of type %v for struct pointer field", FromPy((*C.PyObject)(unsafe.Pointer(pyType)))))
				return -1
			}
			valueWrapper := (*wrapperType)(unsafe.Pointer(value))
			if valueWrapper == nil {
				SetTypeError(fmt.Errorf("invalid value for struct pointer field"))
				return -1
			}
			field.Set(reflect.ValueOf(valueWrapper.goObj))
		}
		return 0
	} else if field.Kind() == reflect.Struct {
		if C.Py_IS_TYPE(value, &C.PyDict_Type) != 0 {
			if !ToValue(FromPy(value), field) {
				SetTypeError(fmt.Errorf("failed to convert dict to %s", field.Type()))
				return -1
			}
		} else {
			pyType := (*C.PyTypeObject)(unsafe.Pointer(value.ob_type))
			if _, ok := maps.typeMetas[(*C.PyObject)(unsafe.Pointer(pyType))]; !ok {
				SetTypeError(fmt.Errorf("invalid value of type %v for struct field", FromPy((*C.PyObject)(unsafe.Pointer(pyType)))))
				return -1
			}
			valueWrapper := (*wrapperType)(unsafe.Pointer(value))
			baseAddr := goPtr.UnsafePointer()
			fieldAddr := unsafe.Add(baseAddr, typeMeta.typ.Field(methodMeta.index).Offset)
			fieldPtr := reflect.NewAt(fieldType, fieldAddr)
			fieldPtr.Elem().Set(reflect.ValueOf(valueWrapper.goObj).Elem())
		}
		return 0
	}

	if !ToValue(FromPy(value), field) {
		SetTypeError(fmt.Errorf("failed to convert value to %s", methodMeta.typ))
		return -1
	}
	return 0
}

//export wrapperMethod
func wrapperMethod(self, args *C.PyObject, methodId C.int) *C.PyObject {
	key := self
	if C.isModule(self) == 0 {
		key = (*C.PyObject)(unsafe.Pointer(self.ob_type))
	}

	maps := getGlobalData()
	typeMeta, ok := maps.typeMetas[key]
	check(ok, fmt.Sprintf("type %v not registered", FromPy(key)))

	methodMeta := typeMeta.methods[uint(methodId)]
	return wrapperMethod_(typeMeta, methodMeta, self, args, methodId)
}

func wrapperMethod_(typeMeta *typeMeta, methodMeta *slotMeta, self, args *C.PyObject, methodId C.int) *C.PyObject {
	methodType := methodMeta.typ
	argc := C.PyTuple_Size(args)
	expectedArgs := methodType.NumIn()
	hasReceiver := methodMeta.hasRecv
	isInit := typeMeta.init == methodMeta

	if hasReceiver {
		expectedArgs-- // decrease expected number if it has a receiver
	}

	if int(argc) != expectedArgs {
		SetTypeError(fmt.Errorf("method %s expects %d arguments, got %d", methodMeta.name, expectedArgs, argc))
		return nil
	}

	goArgs := make([]reflect.Value, methodType.NumIn())
	argIndex := 0

	if hasReceiver {
		wrapper := (*wrapperType)(unsafe.Pointer(self))
		receiverType := methodType.In(0)
		var recv reflect.Value

		if receiverType.Kind() == reflect.Ptr {
			recv = reflect.ValueOf(wrapper.goObj)
		} else {
			recv = reflect.ValueOf(wrapper.goObj).Elem()
		}

		goArgs[0] = recv
		argIndex = 1
	}

	for i := 0; i < int(argc); i++ {
		arg := C.PySequence_GetItem(args, C.Py_ssize_t(i))
		argType := methodType.In(i + argIndex)
		argPy := FromPy(arg)
		goValue := reflect.New(argType).Elem()
		if !ToValue(argPy, goValue) {
			SetTypeError(fmt.Errorf("failed to convert argument %v to %v", argPy, argType))
			return nil
		}
		goArgs[i+argIndex] = goValue
	}

	results := reflect.ValueOf(methodMeta.fn).Call(goArgs)

	// Handle init function return value
	if isInit && !hasReceiver {
		if len(results) == 1 {
			wrapper := (*wrapperType)(unsafe.Pointer(self))
			goObj := reflect.ValueOf(wrapper.goObj).Elem()

			// Handle both pointer and value returns
			result := results[0]
			if result.Type() == reflect.PointerTo(typeMeta.typ) {
				// For pointer constructor, dereference the pointer
				goObj.Set(result.Elem())
			} else {
				// For value constructor
				goObj.Set(result)
			}
			return (*C.PyObject)(unsafe.Pointer(wrapper))
		} else {
			panic("init function without receiver must return the type being created")
		}
	}

	if len(results) == 0 {
		return None().cpyObj()
	}
	if len(results) == 1 {
		return From(results[0].Interface()).cpyObj()
	}

	tuple := MakeTupleWithLen(len(results))
	for i := range results {
		tuple.Set(i, From(results[i].Interface()))
	}
	return tuple.cpyObj()
}

func goNameToPythonName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

func getMethods_(t reflect.Type, methods map[uint]*slotMeta) (ret []C.PyMethodDef) {
	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		if method.IsExported() {
			methodId := uint(len(methods))

			pythonName := goNameToPythonName(method.Name)
			methods[methodId] = &slotMeta{
				name:       method.Name,
				methodName: pythonName,
				fn:         method.Func.Interface(),
				typ:        method.Type,
				hasRecv:    true,
			}

			methodPtr := C.wrapperMethods[methodId]

			ret = append(ret, C.PyMethodDef{
				ml_name:  AllocCStrDontFree(pythonName),
				ml_meth:  (C.PyCFunction)(unsafe.Pointer(methodPtr)),
				ml_flags: C.METH_VARARGS,
				ml_doc:   nil,
			})
		}
	}
	return
}

func getMethods(t reflect.Type, methods map[uint]*slotMeta) *C.PyMethodDef {
	methodsDef := getMethods_(t, methods)
	methodsDef = append(methodsDef, getMethods_(reflect.PointerTo(t), methods)...)
	methodsDef = append(methodsDef, C.PyMethodDef{})
	methodSize := C.size_t(C.sizeof_PyMethodDef * len(methodsDef))
	methodsPtr := (*C.PyMethodDef)(C.malloc(methodSize))
	C.memset(unsafe.Pointer(methodsPtr), 0, methodSize)

	methodArrayPtr := unsafe.Pointer(methodsPtr)
	for i, method := range methodsDef {
		currentMethod := (*C.PyMethodDef)(unsafe.Pointer(uintptr(methodArrayPtr) + uintptr(i)*unsafe.Sizeof(C.PyMethodDef{})))
		*currentMethod = method
	}
	return methodsPtr
}

func getGetsets(t reflect.Type, methods map[uint]*slotMeta) (getsets *C.PyGetSetDef) {
	getsetsList := make([]C.PyGetSetDef, 0)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		pythonName := goNameToPythonName(field.Name)

		// Use getter/setter for all fields
		getId := uint(len(methods))
		methods[getId] = &slotMeta{
			name:       field.Name,
			methodName: pythonName,
			typ:        field.Type,
			hasRecv:    false,
			index:      i,
		}
		setId := uint(len(methods))
		methods[setId] = &slotMeta{
			name:       field.Name,
			methodName: pythonName,
			typ:        field.Type,
			hasRecv:    false,
			index:      i,
		}
		getsetsList = append(getsetsList, C.PyGetSetDef{
			name:    AllocCStrDontFree(pythonName),
			get:     C.getterMethods[getId],
			set:     C.setterMethods[setId],
			doc:     nil,
			closure: nil,
		})
	}

	// Add null terminators
	getsetsList = append(getsetsList, C.PyGetSetDef{})

	// Allocate and copy getsets array
	getsetSize := C.size_t(C.sizeof_PyGetSetDef * len(getsetsList))
	getsetsPtr := (*C.PyGetSetDef)(C.malloc(getsetSize))
	C.memset(unsafe.Pointer(getsetsPtr), 0, getsetSize)

	getsetArrayPtr := unsafe.Pointer(getsetsPtr)
	for i, getset := range getsetsList {
		currentGetSet := (*C.PyGetSetDef)(unsafe.Pointer(uintptr(getsetArrayPtr) + uintptr(i)*unsafe.Sizeof(C.PyGetSetDef{})))
		*currentGetSet = getset
	}

	return getsetsPtr
}

func (m Module) AddType(obj, init any, name, doc string) Object {
	ty := reflect.TypeOf(obj)
	if ty.Kind() == reflect.Ptr {
		ty = ty.Elem()
	}
	if ty.Kind() != reflect.Struct {
		panic("AddType: obj must be a struct or pointer to struct")
	}

	// Check if type already registered
	maps := getGlobalData()
	if pyType, ok := maps.pyTypes[ty]; ok {
		return newObjectRef(pyType)
	}

	meta := &typeMeta{
		typ:     ty,
		methods: make(map[uint]*slotMeta),
	}

	slots := make([]C.PyType_Slot, 0)

	slots = append(slots, C.PyType_Slot{
		slot:  C.Py_tp_alloc,
		pfunc: unsafe.Pointer(C.wrapperAlloc),
	})

	slots = append(slots, C.PyType_Slot{
		slot:  C.Py_tp_dealloc,
		pfunc: unsafe.Pointer(C.wrapperDealloc),
	})

	if init != nil {
		slots = append(slots, C.PyType_Slot{
			slot:  C.Py_tp_init,
			pfunc: unsafe.Pointer(C.wrapperInit),
		})

		initVal := reflect.ValueOf(init)
		initType := initVal.Type()

		if initType.Kind() != reflect.Func {
			panic("Init must be a function")
		}

		// Check if it's a method with receiver
		if initType.NumIn() > 0 &&
			initType.In(0).Kind() == reflect.Ptr &&
			initType.In(0).Elem() == ty {
			// (*T).Init form - pointer receiver
			meta.init = &slotMeta{
				name:       runtime.FuncForPC(initVal.Pointer()).Name(),
				methodName: "__init__",
				fn:         init,
				typ:        initType,
				hasRecv:    true,
			}
		} else if initType.NumOut() == 1 &&
			(initType.Out(0) == ty ||
				(initType.Out(0).Kind() == reflect.Ptr && initType.Out(0).Elem() == ty)) {
			// Constructor function returning T or *T
			meta.init = &slotMeta{
				name:       runtime.FuncForPC(initVal.Pointer()).Name(),
				methodName: "__init__",
				fn:         init,
				typ:        initType,
				hasRecv:    false,
			}
		} else {
			panic("Init function must either have a pointer receiver (*T) or return T/*T")
		}
	}
	getsets := getGetsets(ty, meta.methods)
	slots = append(slots, C.PyType_Slot{slot: C.Py_tp_getset, pfunc: unsafe.Pointer(getsets)})
	slots = append(slots, C.PyType_Slot{slot: C.Py_tp_methods, pfunc: unsafe.Pointer(getMethods(ty, meta.methods))})

	slotCount := len(slots) + 1
	slotSize := C.size_t(C.sizeof_PyType_Slot * slotCount)
	slotsPtr := (*C.PyType_Slot)(C.malloc(slotSize))
	C.memset(unsafe.Pointer(slotsPtr), 0, slotSize)

	slotArrayPtr := unsafe.Pointer(slotsPtr)
	for i, slot := range slots {
		currentSlot := (*C.PyType_Slot)(unsafe.Pointer(uintptr(slotArrayPtr) + uintptr(i)*unsafe.Sizeof(C.PyType_Slot{})))
		*currentSlot = slot
	}

	typeName := fmt.Sprintf("%s.%s", m.Name(), name)

	totalSize := unsafe.Sizeof(wrapperType{})
	spec := &C.PyType_Spec{
		name:      C.CString(typeName),
		basicsize: C.int(totalSize),
		flags:     C.Py_TPFLAGS_DEFAULT,
		slots:     slotsPtr,
	}

	typeObj := C.PyType_FromSpec(spec)
	if typeObj == nil {
		C.free(unsafe.Pointer(spec.name))
		C.free(unsafe.Pointer(slotsPtr))
		panic(fmt.Sprintf("Failed to create type %s", name))
	}

	maps.typeMetas[typeObj] = meta
	maps.pyTypes[ty] = typeObj

	if C.PyModule_AddObjectRef(m.obj, C.CString(name), typeObj) < 0 {
		panic(fmt.Sprintf("Failed to add type %s to module", name))
	}

	// First register any struct field types
	for i := 0; i < ty.NumField(); i++ {
		field := ty.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldType := field.Type
		// Handle pointer types
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		// Recursively register struct types
		if fieldType.Kind() == reflect.Struct {
			maps := getGlobalData()
			if _, ok := maps.pyTypes[fieldType]; !ok {
				// Generate a unique type name based on package path and type name
				nestedName := fieldType.Name()
				m.AddType(reflect.New(fieldType).Elem().Interface(), nil, nestedName, "")
			}
		}
	}

	return newObjectRef(typeObj)
}

func (m Module) AddMethod(name string, fn any, doc string) Func {
	v := reflect.ValueOf(fn)
	t := v.Type()
	if t.Kind() != reflect.Func {
		panic("AddFunction: fn must be a function")
	}
	if name == "" {
		name = runtime.FuncForPC(v.Pointer()).Name()
	}
	if name == "" {
		name = fmt.Sprintf("anonymous_func_%p", fn)
	} else {
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
	}
	name = goNameToPythonName(name)

	hasRecv := false
	if t.NumIn() > 0 {
		firstParam := t.In(0)
		if firstParam.Kind() == reflect.Ptr || firstParam.Kind() == reflect.Interface {
			hasRecv = true
		}
	}

	kwargsType := reflect.TypeOf(KwArgs{})
	hasKwArgs := false
	if t.NumIn() > 0 && t.In(t.NumIn()-1) == kwargsType {
		hasKwArgs = true
	}

	sig := genSig(fn, hasRecv)
	fullDoc := name + sig + "\n--\n\n" + doc
	cDoc := C.CString(fullDoc)

	maps := getGlobalData()
	meta, ok := maps.typeMetas[m.obj]
	if !ok {
		meta = &typeMeta{
			methods: make(map[uint]*slotMeta),
		}
		maps.typeMetas[m.obj] = meta
	}

	methodId := uint(len(meta.methods))

	cName := C.CString(name)

	def := (*C.PyMethodDef)(C.malloc(C.size_t(unsafe.Sizeof(C.PyMethodDef{}))))
	def.ml_name = cName
	def.ml_meth = C.PyCFunction(C.wrapperMethods[methodId])
	def.ml_flags = C.METH_VARARGS
	if hasKwArgs {
		def.ml_flags |= C.METH_KEYWORDS
		def.ml_meth = C.PyCFunction(C.wrapperMethodsWithKwargs[methodId])
	}
	def.ml_doc = cDoc

	methodMeta := &slotMeta{
		name:       name,
		methodName: name,
		fn:         fn,
		typ:        t,
		doc:        fullDoc,
		hasRecv:    hasRecv,
		def:        def,
	}
	meta.methods[methodId] = methodMeta

	pyFunc := C.PyCFunction_NewEx(def, m.obj, m.obj)
	check(pyFunc != nil, fmt.Sprintf("Failed to create function %s", name))

	if C.PyModule_AddObjectRef(m.obj, cName, pyFunc) < 0 {
		C.Py_DecRef(pyFunc)
		panic(fmt.Sprintf("Failed to add function %s to module", name))
	}

	return newFunc(pyFunc)
}

func SetTypeError(err error) {
	errStr := C.CString(err.Error())
	C.PyErr_SetString(C.PyExc_TypeError, errStr)
	C.free(unsafe.Pointer(errStr))
}

// FetchError returns the current Python error as a Go error
func FetchError() error {
	var ptype, pvalue, ptraceback *C.PyObject
	C.PyErr_Fetch(&ptype, &pvalue, &ptraceback)
	if ptype == nil {
		return nil
	}
	defer C.Py_DecRef(ptype)
	if pvalue == nil {
		return fmt.Errorf("python error")
	}
	defer C.Py_DecRef(pvalue)
	if ptraceback != nil {
		defer C.Py_DecRef(ptraceback)
	}

	// Convert error to string
	pyStr := C.PyObject_Str(pvalue)
	if pyStr == nil {
		return fmt.Errorf("python error (failed to get error message)")
	}
	defer C.Py_DecRef(pyStr)

	// Get error message as Go string
	cstr := C.PyUnicode_AsUTF8(pyStr)
	if cstr == nil {
		return fmt.Errorf("python error (failed to decode error message)")
	}

	return fmt.Errorf("python error: %s", C.GoString(cstr))
}

func genSig(fn any, hasRecv bool) string {
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		panic("genSig: fn must be a function")
	}

	var args []string
	startIdx := 0
	if hasRecv {
		startIdx = 1 // skip receiver
	}

	kwargsType := reflect.TypeOf(KwArgs{})
	hasKwArgs := false
	lastParamIdx := t.NumIn() - 1
	if lastParamIdx >= startIdx && t.In(lastParamIdx) == kwargsType {
		hasKwArgs = true
		lastParamIdx-- // don't include KwArgs in regular parameters
	}

	for i := startIdx; i <= lastParamIdx; i++ {
		paramName := fmt.Sprintf("arg%d", i-startIdx)
		args = append(args, paramName)
	}

	// add "/" separator only if there are parameters
	if len(args) > 0 {
		args = append(args, "/")
	}

	// add "**kwargs" if there are keyword arguments
	if hasKwArgs {
		args = append(args, "**kwargs")
	}

	return fmt.Sprintf("(%s)", strings.Join(args, ", "))
}

//export wrapperMethodWithKwargs
func wrapperMethodWithKwargs(self, args, kwargs *C.PyObject, methodId C.int) *C.PyObject {
	key := self
	if C.isModule(self) == 0 {
		key = (*C.PyObject)(unsafe.Pointer(self.ob_type))
	}

	maps := getGlobalData()
	typeMeta, ok := maps.typeMetas[key]
	check(ok, fmt.Sprintf("type %v not registered", FromPy(key)))

	methodMeta := typeMeta.methods[uint(methodId)]
	methodType := methodMeta.typ
	hasReceiver := methodMeta.hasRecv

	expectedArgs := methodType.NumIn()
	if hasReceiver {
		expectedArgs-- // skip receiver
	}
	expectedArgs-- // skip KwArgs

	argc := C.PyTuple_Size(args)
	if int(argc) != expectedArgs {
		SetTypeError(fmt.Errorf("method %s expects %d arguments, got %d", methodMeta.name, expectedArgs, argc))
		return nil
	}

	goArgs := make([]reflect.Value, methodType.NumIn())
	argIndex := 0

	if hasReceiver {
		wrapper := (*wrapperType)(unsafe.Pointer(self))
		receiverType := methodType.In(0)
		var recv reflect.Value

		if receiverType.Kind() == reflect.Ptr {
			recv = reflect.ValueOf(wrapper.goObj)
		} else {
			recv = reflect.ValueOf(wrapper.goObj).Elem()
		}

		goArgs[0] = recv
		argIndex = 1
	}

	for i := 0; i < int(argc); i++ {
		arg := C.PySequence_GetItem(args, C.Py_ssize_t(i))
		argType := methodType.In(i + argIndex)
		argPy := FromPy(arg)
		goValue := reflect.New(argType).Elem()
		if !ToValue(argPy, goValue) {
			SetTypeError(fmt.Errorf("failed to convert argument %v to %v", argPy, argType))
			return nil
		}
		goArgs[i+argIndex] = goValue
	}

	kwargsValue := make(KwArgs)
	if kwargs != nil {
		dict := newDict(kwargs)
		dict.Items()(func(key, value Object) bool {
			kwargsValue[key.String()] = value
			return true
		})
	}
	goArgs[len(goArgs)-1] = reflect.ValueOf(kwargsValue)

	results := reflect.ValueOf(methodMeta.fn).Call(goArgs)

	if len(results) == 0 {
		return None().cpyObj()
	}
	if len(results) == 1 {
		return From(results[0].Interface()).cpyObj()
	}

	tuple := MakeTupleWithLen(len(results))
	for i := range results {
		tuple.Set(i, From(results[i].Interface()))
	}
	return tuple.cpyObj()
}
