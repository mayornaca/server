package tests

import "testing"

// TestClassify covers the cloud-gesvial.20 enriched classifier: distinguishes
// POST_RESPONSE from CARRIER_DELIVERY_RECEIPT and captures the carrier name.
// Backward-compat with IsCarrierConfirmation is verified separately below.
func TestClassify(t *testing.T) {
	tests := []struct {
		name        string
		sender      string
		text        string
		wantType    IncomingType
		wantCarrier string
	}{
		// Post responses must classify as POST_RESPONSE regardless of any
		// carrier name appearing inside (e.g. "Network:ENTEL PCS").
		{
			name:     "full post response",
			sender:   "+56966348648",
			text:     "Firmware Version:JR4G-B53V12\nBattery Voltage:12.6V\nNetwork:ENTEL PCS entel\nSignal level:15",
			wantType: IncomingTypePostResponse,
		},
		{
			name:     "short post response",
			sender:   "+56991517889",
			text:     "Signal Level: 13 Battery Voltage:12.0V",
			wantType: IncomingTypePostResponse,
		},

		// Carrier receipts must classify as CARRIER_DELIVERY_RECEIPT and
		// surface the carrier name so the panel can display it.
		{
			name:        "entel sender",
			sender:      "ENTEL",
			text:        "Tu SMS ha sido entregado correctamente",
			wantType:    IncomingTypeCarrierReceipt,
			wantCarrier: "Entel",
		},
		{
			name:        "movistar mixed case",
			sender:      "MoviStar",
			text:        "Tu mensaje ha sido entregado",
			wantType:    IncomingTypeCarrierReceipt,
			wantCarrier: "Movistar",
		},
		{
			name:        "claro sender lowercase",
			sender:      "claro",
			text:        "Mensaje enviado",
			wantType:    IncomingTypeCarrierReceipt,
			wantCarrier: "Claro",
		},
		{
			name:        "wom body only — sender not branded",
			sender:      "+56900000000",
			text:        "Tu SMS fue entregado por WOM a las 12:34",
			wantType:    IncomingTypeCarrierReceipt,
			wantCarrier: "WOM",
		},
		{
			name:     "english delivery receipt — no carrier id",
			sender:   "12345",
			text:     "Delivery report: message sent successfully",
			wantType: IncomingTypeCarrierReceipt,
		},

		// Unknown — neither pattern matches. Leaves classification absent so
		// the cloud falls back to status-only logic.
		{
			name:     "both empty",
			sender:   "",
			text:     "",
			wantType: IncomingTypeUnknown,
		},
		{
			name:     "random unrelated SMS",
			sender:   "+12025550199",
			text:     "Hello, are you there?",
			wantType: IncomingTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.sender, tt.text)
			if got.Type != tt.wantType {
				t.Errorf("Classify(%q, %q).Type = %v, want %v", tt.sender, tt.text, got.Type, tt.wantType)
			}
			if got.Carrier != tt.wantCarrier {
				t.Errorf("Classify(%q, %q).Carrier = %q, want %q", tt.sender, tt.text, got.Carrier, tt.wantCarrier)
			}
		})
	}
}

func TestIsCarrierConfirmation(t *testing.T) {
	tests := []struct {
		name   string
		sender string
		text   string
		want   bool
	}{
		// Real SOS post responses — must NOT be flagged
		{
			name:   "full format response",
			sender: "+56966348648",
			text:   "Firmware Version:JR4G-B53V12\nBattery Voltage:12.6V\nNetwork:ENTEL PCS entel\nNetwork Mode:4G\nSignal level:15\nTime:11:57",
			want:   false,
		},
		{
			name:   "short format with ENTEL in network",
			sender: "+56991517889",
			text:   "Signal Level: 13 Battery Voltage:12.0V",
			want:   false,
		},
		{
			name:   "chile format with battery voltage",
			sender: "+56942749593",
			text:   "Firmware Version:JR4G-B52V05-CHILE\nBattery Voltage:12.8V\nNetwork Mode:4G\nSignal Level:18",
			want:   false,
		},

		// Carrier delivery receipts — MUST be flagged
		{
			name:   "entel sender",
			sender: "ENTEL",
			text:   "Tu SMS ha sido entregado correctamente",
			want:   true,
		},
		{
			name:   "claro sender lowercase",
			sender: "claro",
			text:   "Mensaje enviado",
			want:   true,
		},
		{
			name:   "movistar sender mixed case",
			sender: "MoviStar",
			text:   "Tu mensaje ha sido entregado",
			want:   true,
		},
		{
			name:   "wom delivery receipt body only",
			sender: "+56900000000",
			text:   "Tu SMS fue entregado a las 12:34",
			want:   true,
		},
		{
			name:   "delivery receipt english",
			sender: "12345",
			text:   "Delivery report: message sent successfully",
			want:   true,
		},
		{
			name:   "mensaje entregado correctamente",
			sender: "Network",
			text:   "Tu mensaje fue entregado correctamente",
			want:   true,
		},

		// Edge cases
		{
			name:   "both empty",
			sender: "",
			text:   "",
			want:   false,
		},
		{
			name:   "post phone with carrier name in network only — no body match",
			sender: "+56923878926",
			text:   "Network:ENTEL PCS entel\nNetwork Mode:4G",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCarrierConfirmation(tt.sender, tt.text)
			if got != tt.want {
				t.Errorf("IsCarrierConfirmation(%q, %q) = %v, want %v", tt.sender, tt.text, got, tt.want)
			}
		})
	}
}
