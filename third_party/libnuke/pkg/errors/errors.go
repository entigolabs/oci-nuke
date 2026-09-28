// Package errors provides common errors that can be used throughout the library for handling of resource errors
package errors

import "errors"

type ErrSkipRequest string

func (err ErrSkipRequest) Error() string {
	return string(err)
}

type ErrUnknownEndpoint string

func (err ErrUnknownEndpoint) Error() string {
	return string(err)
}

type ErrWaitResource string

func (err ErrWaitResource) Error() string {
	return string(err)
}

type ErrHoldResource string

func (err ErrHoldResource) Error() string {
	return string(err)
}

// entigo patch: ErrDeferResource marks a resource that cannot proceed in this run at all,
// only in a later one. Unlike ErrHoldResource, which is retried within the run, the item is
// left in ItemStateDeferred and neither retried nor counted as failed, so a run that leaves
// only deferred items behind still succeeds.
type ErrDeferResource string

func (err ErrDeferResource) Error() string {
	return string(err)
}

type ErrUnknownPreset string

func (err ErrUnknownPreset) Error() string {
	return string(err)
}

type ErrDeprecatedResourceType string

func (err ErrDeprecatedResourceType) Error() string {
	return string(err)
}

var ErrNoBlocklistDefined = errors.New("no blocklist defined")
var ErrBlocklistAccount = errors.New("account is in blocklist")
var ErrAccountNotConfigured = errors.New("account is not configured")
