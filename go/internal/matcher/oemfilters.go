package matcher

// oemFilters mirrors the Filter_1..Filter_22/filter_list tables in
// State::genmarker (matcher.cpp): for each canonical OEM brand name (the
// first entry), the alternate spellings/substrings to search for in a
// system's motherboard manufacturer string. Extracted programmatically from
// the C source, same as OSDecorations.
var oemFilters = [22][]string{
	{"Acer", "acer", "emachines", "packard", "bell", "gateway", "aspire"}, // Acer
	{"Apple", "apple"}, // Apple
	{"Asus", "asus"},   // Asus
	{"OEM", "clevo", "eurocom", "sager", "iru", "viewsonic", "viewbook"}, // OEM
	{"Dell", "dell", "alienware", "arima", "jetway", "gericom"},          // Dell
	{"Fujitsu", "fujitsu", "sieme"},                                      // Fujitsu
	{"OEM", "ecs", "elitegroup", "roverbook", "rover", "shuttle"},        // OEM
	{"HP", "hp", "hewle", "compaq"},                                      // HP
	{"OEM", "intel", "wistron"},                                          // OEM
	{"Lenovo", "lenovo", "compal", "ibm"},                                // Lenovo
	{"LG", "lg"},                                                         // LG
	{"OEM", "mitac", "mtc", "depo", "getac"},                             // OEM
	{"MSI", "msi", "micro-star"},                                         // MSI
	{"Panasonic", "panasonic", "matsushita"},                             // Panasonic
	{"OEM", "quanta", "prolink", "nec", "k-systems", "benq", "vizio"},    // OEM
	{"OEM", "pegatron", "medion"},                                        // OEM
	{"Samsung", "samsung"},                                               // Samsung
	{"Gigabyte", "gigabyte"},                                             // Gigabyte
	{"Sony", "sony", "vaio"},                                             // Sony
	{"Toshiba", "toshiba"},                                               // Toshiba
	{"OEM", "twinhead", "durabook"},                                      // OEM
	{"NEC", "Nec_brand", "nec"},                                          // NEC
}
