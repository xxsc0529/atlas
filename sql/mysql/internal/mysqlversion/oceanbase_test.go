// Copyright 2021-present The Atlas Authors. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package mysqlversion_test

import (
	"testing"

	"ariga.io/atlas/sql/mysql/internal/mysqlversion"
)

func TestV_OceanBase(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"5.7.25-OceanBase-v3.2.3", true},
		{"5.7.25-OceanBase_CE-v4.0.0", true},
		{"5.6.50-OceanBase-v4.1.0", true},
		{"8.0.25-OceanBase-v4.2.0", true},
		{"5.7.32-TiDB-v5.4.0", false},
		{"5.7.32-MariaDB-10.5.12", false},
		{"8.0.28", false},
		{"5.7.36", false},
	}
	for _, tt := range tests {
		t.Run(tt.v, func(t *testing.T) {
			if got := mysqlversion.V(tt.v).OceanBase(); got != tt.want {
				t.Errorf("V.OceanBase() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestV_Compare_OceanBase(t *testing.T) {
	tests := []struct {
		v    string
		w    string
		want int
	}{
		{"5.7.25-OceanBase-v3.2.3", "5.7.25", 0},
		{"5.7.25-OceanBase-v3.2.3", "5.7.26", -1},
		{"5.7.25-OceanBase-v3.2.3", "5.7.24", 1},
		{"8.0.25-OceanBase-v4.2.0", "8.0.25", 0},
		{"8.0.25-OceanBase-v4.2.0", "8.0.26", -1},
		{"8.0.25-OceanBase-v4.2.0", "8.0.24", 1},
	}
	for _, tt := range tests {
		t.Run(tt.v+" vs "+tt.w, func(t *testing.T) {
			if got := mysqlversion.V(tt.v).Compare(tt.w); got != tt.want {
				t.Errorf("V.Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}