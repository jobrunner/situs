package domain

// SchemeWGSRPDL3 is the only area scheme situs stores today: WGSRPD level 3
// ("botanical countries"), which is what hostus reports per concept.
const SchemeWGSRPDL3 = "wgsrpd_l3"

// Area is a distribution area. Scheme and Code together identify it — a bare
// code is ambiguous across schemes, the same way a habitat type code is
// ambiguous across typologies.
type Area struct {
	Scheme string
	Code   string
}

func (a Area) String() string { return a.Scheme + ":" + a.Code }

// IsComplete reports whether both halves are present. An incomplete area must
// never reach the index: it would silently match nothing.
func (a Area) IsComplete() bool { return a.Scheme != "" && a.Code != "" }
