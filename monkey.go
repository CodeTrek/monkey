package monkey // import "bou.ke/monkey"

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"
)

// patch is an applied patch
// needed to undo a patch
type patch struct {
	originalBytes []byte
	replacement   *reflect.Value
	aliasPatchedPos  uintptr
	aliasOriginalBytes []byte
	addr *uintptr
}

var (
	lock = sync.Mutex{}

	patches = make(map[uintptr]patch)
)

type value struct {
	_   uintptr
	ptr unsafe.Pointer
}

func getPtr(v reflect.Value) unsafe.Pointer {
	return (*value)(unsafe.Pointer(&v)).ptr
}

type PatchGuard struct {
	target      reflect.Value
	replacement reflect.Value
	alias		reflect.Value	// Use this interface to access the original target
}

func (g *PatchGuard) Unpatch() {
	unpatchValue(g.target)
}

func (g *PatchGuard) Restore() {
	patchValue(g.target, g.replacement, g.alias)
}

// Patch replaces a function with another
// alias: A wrapper, to access the original target when patched
func Patch(target, alias, replacement interface{}) *PatchGuard {
	t := reflect.ValueOf(target)
	r := reflect.ValueOf(replacement)
	a := reflect.ValueOf(alias)
	patchValue(t, r, a)

	return &PatchGuard{t, r, a}
}

// PatchInstanceMethod replaces an instance method methodName for the type target with replacement
// Replacement should expect the receiver (of type target) as the first argument
func PatchInstanceMethod(target reflect.Type, methodName string, alias, replacement interface{}) *PatchGuard {
	m, ok := target.MethodByName(methodName)
	if !ok {
		panic(fmt.Sprintf("unknown method %s", methodName))
	}
	r := reflect.ValueOf(replacement)
	a := reflect.ValueOf(alias)
	patchValue(m.Func, r, a)

	return &PatchGuard{m.Func, r, a}
}

func patchValue(target, replacement, alias reflect.Value) {
	lock.Lock()
	defer lock.Unlock()

	if target.Kind() != reflect.Func {
		panic("target has to be a Func")
	}

	if replacement.Kind() != reflect.Func {
		panic("replacement has to be a Func")
	}

	if alias.Kind() != reflect.Func {
		panic("alias has to be a Func")
	}

	if target.Type() != replacement.Type() {
		panic(fmt.Sprintf("target and replacement have to have the same type %s != %s", target.Type(), replacement.Type()))
	}

	if target.Type() != alias.Type() {
		panic(fmt.Sprintf("target and alias have to have the same type %s != %s", target.Type(), alias.Type()))
	}

	if patch, ok := patches[target.Pointer()]; ok {
		unpatch(target.Pointer(), patch)
	}

	addr := new(uintptr)
	*addr = *(*uintptr)(getPtr(target)) + codeOffset()

	aliasBytes := replaceFunction(alias.Pointer() + codeOffset(), (uintptr)(unsafe.Pointer(addr)))
	bytes := replaceFunction(target.Pointer(), (uintptr)(getPtr(replacement)))
	patches[target.Pointer()] = patch{bytes, &replacement, alias.Pointer() + codeOffset(), aliasBytes, addr}
}

// Unpatch removes any monkey patches on target
// returns whether target was patched in the first place
func Unpatch(target interface{}) bool {
	return unpatchValue(reflect.ValueOf(target))
}

// UnpatchInstanceMethod removes the patch on methodName of the target
// returns whether it was patched in the first place
func UnpatchInstanceMethod(target reflect.Type, methodName string) bool {
	m, ok := target.MethodByName(methodName)
	if !ok {
		panic(fmt.Sprintf("unknown method %s", methodName))
	}
	return unpatchValue(m.Func)
}

// UnpatchAll removes all applied monkeypatches
func UnpatchAll() {
	lock.Lock()
	defer lock.Unlock()
	for target, p := range patches {
		unpatch(target, p)
		delete(patches, target)
	}
}

// Unpatch removes a monkeypatch from the specified function
// returns whether the function was patched in the first place
func unpatchValue(target reflect.Value) bool {
	lock.Lock()
	defer lock.Unlock()
	patch, ok := patches[target.Pointer()]
	if !ok {
		return false
	}
	unpatch(target.Pointer(), patch)
	delete(patches, target.Pointer())
	return true
}

func unpatch(target uintptr, p patch) {
	copyToLocation(target, p.originalBytes)
	copyToLocation(p.aliasPatchedPos, p.aliasOriginalBytes)
}
