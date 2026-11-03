package main

import (
	"encoding/base64"
	"testing"
)

func TestValidPAMPaymentMethods(t *testing.T) {
	tests := []struct {
		name    string
		methods []pamPaymentMethod
		valid   bool
	}{
		{
			name: "active configured methods",
			methods: []pamPaymentMethod{
				{Code: "bank_transfer", Name: "Transfer Bank", Active: true},
				{Code: "qris", Name: "QRIS", Instructions: "Gunakan QRIS resmi."},
			},
			valid: true,
		},
		{name: "no methods", methods: []pamPaymentMethod{}, valid: false},
		{name: "no active methods", methods: []pamPaymentMethod{{Code: "cash", Name: "Tunai"}}, valid: false},
		{name: "duplicate codes", methods: []pamPaymentMethod{{Code: "qris", Name: "QRIS", Active: true}, {Code: "qris", Name: "QRIS lain", Active: true}}, valid: false},
		{name: "invalid code", methods: []pamPaymentMethod{{Code: "QRIS!", Name: "QRIS", Active: true}}, valid: false},
		{name: "missing name", methods: []pamPaymentMethod{{Code: "qris", Active: true}}, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validPAMPaymentMethods(test.methods); got != test.valid {
				t.Fatalf("validPAMPaymentMethods() = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestPAMBillTotalChecksOverflow(t *testing.T) {
	tests := []struct {
		name        string
		consumption int64
		rate        int64
		fee         int64
		total       int64
		valid       bool
	}{
		{name: "normal", consumption: 12, rate: 2500, fee: 5000, total: 35000, valid: true},
		{name: "fee only", consumption: 0, rate: 100, fee: 500, total: 500, valid: true},
		{name: "negative input", consumption: -1, rate: 1, fee: 0, valid: false},
		{name: "overflow", consumption: 1<<62 - 1, rate: 3, fee: 0, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := pamBillTotal(test.consumption, test.rate, test.fee)
			if got != test.total || valid != test.valid {
				t.Fatalf("pamBillTotal() = (%d, %v), want (%d, %v)", got, valid, test.total, test.valid)
			}
		})
	}
}

func TestValidatePAMImageData(t *testing.T) {
	validPNG := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	tests := []struct {
		name string
		data string
		ok   bool
	}{
		{name: "empty optional QR image", data: "", ok: true},
		{name: "PNG", data: validPNG, ok: true},
		{name: "unsupported SVG", data: "data:image/svg+xml;base64,PHN2Zz4=", ok: false},
		{name: "mismatched content type", data: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10}), ok: false},
		{name: "invalid encoding", data: "data:image/png;base64,not-valid", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePAMImageData(test.data)
			if (err == nil) != test.ok {
				t.Fatalf("validatePAMImageData() error = %v, want valid=%v", err, test.ok)
			}
		})
	}
}
