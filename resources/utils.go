package resources

// strPtrOrNil returns nil for an empty pagination cursor, otherwise a pointer to it -
// OCI list requests expect a nil Page on the first call, not a pointer to "".
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// strOr dereferences s, or returns fallback when the service left it out.
func strOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}
