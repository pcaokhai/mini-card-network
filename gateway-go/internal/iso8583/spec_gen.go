// Code generated from contracts/iso8583/packager-spec.yaml by internal/iso8583/gen. DO NOT EDIT.
package iso8583

// Fields is the MCN-87A field table (docs/03 §3).
var Fields = map[int]FieldSpec{
	1:   {Number: 1, Type: "b", Length: 8, Prefix: "", Name: "Secondary bitmap"},                // Secondary bitmap
	2:   {Number: 2, Type: "n", Length: 19, Prefix: "LL", Name: "PAN"},                          // PAN
	3:   {Number: 3, Type: "n", Length: 6, Prefix: "", Name: "Processing code"},                 // Processing code
	4:   {Number: 4, Type: "n", Length: 12, Prefix: "", Name: "Amount transaction"},             // Amount transaction
	7:   {Number: 7, Type: "n", Length: 10, Prefix: "", Name: "Transmission date time"},         // Transmission date time
	11:  {Number: 11, Type: "n", Length: 6, Prefix: "", Name: "STAN"},                           // STAN
	12:  {Number: 12, Type: "n", Length: 6, Prefix: "", Name: "Time local transaction"},         // Time local transaction
	13:  {Number: 13, Type: "n", Length: 4, Prefix: "", Name: "Date local transaction"},         // Date local transaction
	14:  {Number: 14, Type: "n", Length: 4, Prefix: "", Name: "Expiration date"},                // Expiration date
	15:  {Number: 15, Type: "n", Length: 4, Prefix: "", Name: "Settlement date"},                // Settlement date
	18:  {Number: 18, Type: "n", Length: 4, Prefix: "", Name: "Merchant category code"},         // Merchant category code
	22:  {Number: 22, Type: "n", Length: 3, Prefix: "", Name: "POS entry mode"},                 // POS entry mode
	25:  {Number: 25, Type: "n", Length: 2, Prefix: "", Name: "POS condition code"},             // POS condition code
	32:  {Number: 32, Type: "n", Length: 11, Prefix: "LL", Name: "Acquiring institution ID"},    // Acquiring institution ID
	37:  {Number: 37, Type: "an", Length: 12, Prefix: "", Name: "RRN"},                          // RRN
	38:  {Number: 38, Type: "an", Length: 6, Prefix: "", Name: "Authorization ID response"},     // Authorization ID response
	39:  {Number: 39, Type: "an", Length: 2, Prefix: "", Name: "Response code"},                 // Response code
	41:  {Number: 41, Type: "ans", Length: 8, Prefix: "", Name: "Terminal ID"},                  // Terminal ID
	42:  {Number: 42, Type: "ans", Length: 15, Prefix: "", Name: "Merchant ID"},                 // Merchant ID
	43:  {Number: 43, Type: "ans", Length: 40, Prefix: "", Name: "Card acceptor name location"}, // Card acceptor name location
	48:  {Number: 48, Type: "ans", Length: 999, Prefix: "LLL", Name: "Additional data private"}, // Additional data private
	49:  {Number: 49, Type: "n", Length: 3, Prefix: "", Name: "Currency code"},                  // Currency code
	52:  {Number: 52, Type: "b", Length: 8, Prefix: "", Name: "PIN block"},                      // PIN block
	53:  {Number: 53, Type: "n", Length: 16, Prefix: "", Name: "Security control info"},         // Security control info
	54:  {Number: 54, Type: "an", Length: 120, Prefix: "LLL", Name: "Additional amounts"},       // Additional amounts
	55:  {Number: 55, Type: "b", Length: 255, Prefix: "LLL", Name: "ICC data"},                  // ICC data
	64:  {Number: 64, Type: "b", Length: 8, Prefix: "", Name: "MAC"},                            // MAC
	70:  {Number: 70, Type: "n", Length: 3, Prefix: "", Name: "Network management code"},        // Network management code
	74:  {Number: 74, Type: "n", Length: 10, Prefix: "", Name: "Credits number"},                // Credits number
	75:  {Number: 75, Type: "n", Length: 10, Prefix: "", Name: "Credits reversal number"},       // Credits reversal number
	76:  {Number: 76, Type: "n", Length: 10, Prefix: "", Name: "Debits number"},                 // Debits number
	77:  {Number: 77, Type: "n", Length: 10, Prefix: "", Name: "Debits reversal number"},        // Debits reversal number
	86:  {Number: 86, Type: "n", Length: 16, Prefix: "", Name: "Credits amount"},                // Credits amount
	87:  {Number: 87, Type: "n", Length: 16, Prefix: "", Name: "Credits reversal amount"},       // Credits reversal amount
	88:  {Number: 88, Type: "n", Length: 16, Prefix: "", Name: "Debits amount"},                 // Debits amount
	89:  {Number: 89, Type: "n", Length: 16, Prefix: "", Name: "Debits reversal amount"},        // Debits reversal amount
	90:  {Number: 90, Type: "n", Length: 42, Prefix: "", Name: "Original data elements"},        // Original data elements
	97:  {Number: 97, Type: "an", Length: 17, Prefix: "", Name: "Net settlement amount"},        // Net settlement amount
	128: {Number: 128, Type: "b", Length: 8, Prefix: "", Name: "MAC secondary"},                 // MAC secondary
}
