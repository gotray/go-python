# Design

## Python types wrapper design

To automatically DecRef Python objects, we need to wrap them in a Go struct that will call DecRef when it is garbage collected. This is done by embedding a PyObject in a Go struct and registering a finalizer on the Go struct. Below is an example of how this is done:

```go
type pyObject struct {
  obj *C.PyObject
}

func newObject(obj *C.PyObject) *pyObject {
  o := &pyObject{obj}
  runtime.SetFinalizer(o, func(o *pyObject) {
    C.Py_DecRef(o.obj)
  })
  return o
}
```

To wrap generic PyObject(s) to typed Python objects, the best way is using alias types. Below is an example of how this is done:

```go
type Object *pyObject

func (o Object) GetAttrString(name string) Object {
  return newObject(o.obj.GetAttrString(name))
}

type Dict Object

func (d Dict) SetItemString(name string, value Object) {
  d.obj.SetItemString(name, value.obj)
}
```

Unfortunately, Go does not allow defining methods on alias types like the above.

```shell
invalid receiver type PyObject (pointer or interface type)
invalid receiver type PyDict (pointer or interface type)
```

We can define a new type that embeds the alias type and define methods on the new type. Below is an example of how this is done:

```go
type Object struct {
  *pyObject
}

func (o *Object) GetAttrString(name string) *Object {
  return &Object{newObject(o.obj.GetAttrString(name))}
}

type Dict struct {
  *Object
}

func (d *Dict) SetItemString(name string, value *Object) {
  d.obj.SetItemString(name, value.obj)
}
```

But allocating a `PyDict` object will allocate a `PyObject` object and a `pyObject` object. This is not efficient.

We can use a `struct` instead of a `pointer` to avoid this. Below is an example of how this is done:

```go
type Object struct {
  *pyObject
}

func (o Object) GetAttrString(name string) Object {
  return Object{newObject(o.obj.GetAttrString(name))}
}

type Dict struct {
  Object
}

func (d Dict) SetItemString(name string, value Object) {
  d.obj.SetItemString(name, value.obj)
}
```
