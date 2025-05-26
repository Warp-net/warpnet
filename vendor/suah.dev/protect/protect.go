/*
Package protect is a wrapper for OpenBSD's pledge(2) and unveil(2) system
calls.

This library is trivial, but I found myself writing it often enough that I
figure it should be a package.
*/
package protect

import (
	"regexp"
	"strings"
)

// Unveil is a wrapper for OpenBSD's unveil(2). unveil can be used to limit
// a processes view of the filesystem.
//
// The first call to Unveil removes a processes visibility to everything
// except 'path'. Any subsequent calls expand the view to contain those
// paths. Finally a call to UnveilBlock will lock the view in place.
// Preventing access to anything else.
//
// On non-OpenBSD machines this call is a noop.
func Unveil(path string, flags string) error {
	return unveil(path, flags)
}

// UnveilSet takes a set of Unveils and runs them all, returning the first
// error encountered. Optionally call UnveilBlock at the end.
func UnveilSet(set map[string]string, block bool) error {
	for p, s := range set {
		err := Unveil(p, s)
		if err != nil {
			return err
		}
	}

	if block {
		return UnveilBlock()
	}

	return nil
}

// UnveilBlock locks the Unveil'd paths. Preventing further changes to a
// processes filesystem view.
//
// On non-OpenBSD machines this call is a noop.
func UnveilBlock() error {
	return unveilBlock()
}

// Pledge wraps OpenBSD's pledge(2) system call. One can use this to limit
// the system calls a process can make.
//
// On non-OpenBSD machines this call is a noop.
func Pledge(promises string) error {
	return pledge(promises)
}

// ReducePledges takes the current list of plpedges and a list of pledges that
// should be removed. The new list is returned and Pledge() will be called
// with the reduced set of pledges.
func ReducePledges(current, toRemove string) (string, error) {
	newPledges, err := reduce(current, toRemove)
	if err != nil {
		return "", err
	}

	return newPledges, pledge(newPledges)
}

func reduce(a, b string) (string, error) {
	var newList []string
	currentList := strings.Split(a, " ")

	for _, s := range currentList {
		match, err := regexp.MatchString(s, b)
		if err != nil {
			return "", err
		}

		if !match {
			newList = append(newList, s)
		}
	}

	return strings.Join(newList, " "), nil
}
