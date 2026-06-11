// Package apps is the umbrella for the P3 application-lifecycle module.
// It currently holds only the shared error-mapping seam used by sub-packages
// like apps/repo, apps/application, and apps/release. The real implementation
// of MapError ships in Task 9 (Batch 4); this file exists so downstream code
// can compile and ship ahead of T9.
package apps

// MapError normalises an upstream error (helm SDK, OCI registry HTTP, chart
// repository transport) into an apperr.BizError in the 42101-42199 range.
// Until T9 lands the real classifier, callers get the raw error back unchanged
// so the call site behaves the same as before — only the category label
// changes once T9 ships.
func MapError(err error) error { return err }
