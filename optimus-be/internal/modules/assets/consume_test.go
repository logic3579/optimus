package assets

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"testing"

	apperr "optimus-be/internal/infra/errors"

	"github.com/stretchr/testify/require"
)

func TestErrAssetsInstanceNotFoundSupportsErrorsIs(t *testing.T) {
	require.Error(t, ErrAssetsInstanceNotFound)
	require.True(t, errors.Is(fmt.Errorf("consumer: %w", ErrAssetsInstanceNotFound), ErrAssetsInstanceNotFound))
}

func TestConsumerRejectsInvalidInputsBeforeQuerying(t *testing.T) {
	c := NewConsumer(nil)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "invalid IP", call: func() error {
			_, err := c.LookupInstanceByPrivateIP(context.Background(), netip.Addr{})
			return err
		}},
		{name: "zero account ID for lookup", call: func() error {
			_, err := c.LookupInstanceByID(context.Background(), 0, "us-east-1", "i-a")
			return err
		}},
		{name: "negative account ID for lookup", call: func() error {
			_, err := c.LookupInstanceByID(context.Background(), -1, "us-east-1", "i-a")
			return err
		}},
		{name: "blank lookup region", call: func() error {
			_, err := c.LookupInstanceByID(context.Background(), 1, " \t", "i-a")
			return err
		}},
		{name: "blank instance ID", call: func() error {
			_, err := c.LookupInstanceByID(context.Background(), 1, "us-east-1", "\n")
			return err
		}},
		{name: "zero account ID for list", call: func() error {
			_, err := c.ListInstancesByVPC(context.Background(), 0, "us-east-1", "vpc-a")
			return err
		}},
		{name: "negative account ID for list", call: func() error {
			_, err := c.ListInstancesByVPC(context.Background(), math.MinInt64, "us-east-1", "vpc-a")
			return err
		}},
		{name: "blank list region", call: func() error {
			_, err := c.ListInstancesByVPC(context.Background(), math.MaxInt64, "", "vpc-a")
			return err
		}},
		{name: "blank VPC ID", call: func() error {
			_, err := c.ListInstancesByVPC(context.Background(), math.MaxInt64, "us-east-1", " ")
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			var biz *apperr.BizError
			require.ErrorAs(t, err, &biz)
			require.Equal(t, apperr.CodeValidation, biz.Code)
		})
	}
}

func TestParseProjectedIPNormalizesAddresses(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
		want  string
	}{
		{name: "null", valid: false},
		{name: "IPv4", value: "10.0.0.5", valid: true, want: "10.0.0.5"},
		{name: "IPv4 mapped IPv6", value: "::ffff:10.0.0.5", valid: true, want: "10.0.0.5"},
		{name: "IPv6", value: "2001:0db8:0:0::1", valid: true, want: "2001:db8::1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseProjectedIP(test.value, test.valid)
			require.Equal(t, test.valid, got.IsValid())
			if test.valid {
				require.Equal(t, test.want, got.String())
			}
		})
	}
}
