package models

import (
	"database/sql/driver"
	"fmt"

	"github.com/lib/pq"
)

type StringArray []string

func (a StringArray) Value() (driver.Value, error) { return pq.StringArray(a).Value() }

func (a *StringArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	pa := pq.StringArray{}
	if err := pa.Scan(src); err != nil {
		return fmt.Errorf("StringArray.Scan: %w", err)
	}
	*a = StringArray(pa)
	return nil
}
