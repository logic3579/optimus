package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	apperr "optimus-be/internal/infra/errors"
)

// readValuesFromTgz returns the content of the file at <root>/values.yaml
// inside a chart tarball, where <root> is the first directory entry. A chart
// without a values.yaml is valid (rare); the function returns ("", nil) in
// that case so callers can serve an empty string back to clients.
func readValuesFromTgz(tgz []byte) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", err.Error())
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", err.Error())
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Match exactly <root>/values.yaml (no nested subchart values).
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) != 2 || parts[1] != "values.yaml" {
			continue
		}
		buf, err := io.ReadAll(tr)
		if err != nil {
			return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", err.Error())
		}
		return string(buf), nil
	}
	return "", nil
}
