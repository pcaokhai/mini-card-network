package lab

// easyNameOverrides holds the DEs whose packager-spec.yaml `name` isn't learner-friendly on
// its own (docs/06 MCN-103 ruling). Every other DE's easyName equals its technicalName.
var easyNameOverrides = map[int]string{
	22: "Entry mode",
	55: "Card chip data (EMV)",
	90: "Reference to the original message",
}

func easyName(de int, technicalName string) string {
	if override, ok := easyNameOverrides[de]; ok {
		return override
	}
	return technicalName
}
