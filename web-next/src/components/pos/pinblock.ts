// docs/03 §11: PIN block = '0' + PIN length (hex) + PIN + 'F' padding, XOR '0000' + 12 rightmost
// PAN digits excluding the check digit. This is a real ISO 9564-1 format-0 block; only the
// "encryption" step downstream uses a fixed, publicly-documented simulator key - never a real TPK.
export function buildPinBlock(pin: string, pan: string): string {
  const pinField = ("0" + pin.length.toString(16) + pin).padEnd(16, "F");
  const panDigits = pan.slice(0, -1).slice(-12); // 12 rightmost digits excluding the check digit
  const panField = "0000" + panDigits;

  let result = "";
  for (let i = 0; i < 16; i++) {
    const a = parseInt(pinField[i], 16);
    const b = parseInt(panField[i], 16);
    result += (a ^ b).toString(16);
  }
  return result.toUpperCase();
}

// Lab-only: a fixed, documented, non-secret "simulator TPK" - never a real production key.
// A real POS's TPK is provisioned by an HSM and never appears in client-readable code.
const SIMULATOR_TPK_HEX = "0123456789ABCDEF";

export async function encryptPinBlockForSimulator(pin: string, pan: string): Promise<string> {
  const block = buildPinBlock(pin, pan);
  const keyBytes = hexToBytes(SIMULATOR_TPK_HEX);
  const blockBytes = hexToBytes(block);
  const xored = blockBytes.map((b, i) => b ^ keyBytes[i]);
  return bytesToHex(xored);
}

function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) bytes[i] = parseInt(hex.substr(i * 2, 2), 16);
  return bytes;
}

function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("")
    .toUpperCase();
}
