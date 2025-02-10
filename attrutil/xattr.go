package attrutil

import (
	"strings"

	"github.com/pkg/xattr"
)

// osXattr implements the Attr interface using OS-specific extended attributes.
type osXattr struct {
	ns string
}

// Xattr returns a new instance of osXattr.
func Xattr() Attr {
	return &osXattr{}
}

// name returns the full attribute name including the namespace.
func (a *osXattr) name(name string) string {
	if a.ns == "" {
		return name
	}
	if name == "" {
		return a.ns
	}
	return a.ns + "." + name
}

// List lists the extended attribute keys for the given path.
func (a *osXattr) List(path string) (keys []string, err error) {
	var allKeys []string
	if allKeys, err = xattr.List(path); err != nil {
		return
	}
	prefix := ""
	if a.ns != "" {
		prefix = a.ns + "."
	}
	for _, key := range allKeys {
		if strings.HasPrefix(key, prefix) {
			key = strings.TrimPrefix(key, prefix)
			keys = append(keys, key)
		}
	}
	return
}

// Get retrieves the value of the extended attribute for the given path and name.
func (a *osXattr) Get(path string, name string) (value []byte, err error) {
	return xattr.Get(path, a.name(name))
}

// Set sets the value of the extended attribute for the given path and name.
func (a *osXattr) Set(path string, name string, value []byte) (err error) {
	return xattr.Set(path, a.name(name), value)
}

// GetAttrs retrieves all extended attributes for the given path.
func (a *osXattr) GetAttrs(path string) (attrs map[string][]byte, err error) {
	var keys []string
	if keys, err = a.List(path); err != nil {
		return
	}
	attrs = make(map[string][]byte, len(keys))
	for _, key := range keys {
		if attrs[key], err = xattr.Get(path, a.name(key)); err != nil {
			return
		}
	}
	return
}

// SetAttrs sets multiple extended attributes for the given path.
func (a *osXattr) SetAttrs(path string, attrs map[string][]byte) (err error) {
	var keys []string
	if keys, err = a.List(path); err != nil {
		return
	}
	for _, key := range keys {
		if _, found := attrs[key]; !found {
			if err = xattr.Remove(path, a.name((key))); err != nil {
				return
			}
		}
	}
	for key, value := range attrs {
		if err = xattr.Set(path, a.name(key), value); err != nil {
			return
		}
	}
	return
}

// ListNS lists the namespaces of extended attributes for the given path.
func (a *osXattr) ListNS(path string) (namespaces []string, err error) {
	var keys []string
	if keys, err = xattr.List(path); err != nil {
		return
	}
	prefix := ""
	if a.ns != "" {
		prefix = a.ns + "."
	}
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			key = strings.TrimPrefix(key, prefix)
			if parts := strings.SplitN(key, ".", 2); len(parts) == 2 {
				namespaces = append(namespaces, parts[0])
			}
		}
	}
	if namespaces == nil {
		return []string{}, nil
	}
	return
}

// NS returns a new Attr instance with the specified namespace.
func (a *osXattr) NS(ns string) Attr {
	ns = strings.Trim(ns, ".")
	if a.ns == "" {
		return &osXattr{ns: ns}
	}
	return &osXattr{ns: a.ns + "." + ns}
}

// NSName returns the current namespace.
func (a *osXattr) NSName() string {
	return a.ns
}

// Delete removes the extended attribute for the given path and name.
func (a *osXattr) Delete(path string, name string) (err error) {
	return xattr.Remove(path, a.name(name))
}

// DeleteNS removes the namespace of extended attributes for the given path.
func (a *osXattr) DeleteNS(path string, ns string) (err error) {
	var keys []string
	if keys, err = a.List(path); err != nil {
		return
	}
	prefix := a.name(ns) + "."
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			if err = xattr.Remove(path, a.name(key)); err != nil {
				return
			}
		}
	}
	return
}

// Attr defines an interface for managing extended attributes.
type Attr interface {
	List(path string) (keys []string, err error)
	Get(path string, name string) (value []byte, err error)
	Set(path string, name string, value []byte) (err error)
	GetAttrs(path string) (attrs map[string][]byte, err error)
	SetAttrs(path string, attrs map[string][]byte) (err error)
	ListNS(path string) (namespaces []string, err error)
	Delete(path string, name string) (err error)
	DeleteNS(path string, ns string) (err error)
	NS(ns string) Attr
	NSName() string
}
