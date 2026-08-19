package patch

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// TestFaultCarriesAStackFromItsOrigin is the property the constructors exist for.
// Eleven places return a fault; the stack has to name which, and it can only do
// that if it was captured where the fault was made.
func TestFaultCarriesAStackFromItsOrigin(t *testing.T) {
	g := NewWithT(t)
	_, err := Parse("[a/b.ts#1A2B]\nPUT 12.=12:\n-old")

	g.Expect(err).To(HaveOccurred())
	rendered := strings.ReplaceAll(fmt.Sprintf("%+v", err), "\t", " ")
	g.Expect(rendered).To(ContainSubstring("parse.go"), "the stack names the origin file")
	g.Expect(rendered).To(ContainSubstring("patch.faultAt"), "captured at construction")
}
