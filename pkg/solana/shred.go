package solana

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Legacy data shreds are always zero padded and
// the same size as coding shreds.
//
// Merkle shreds sign merkle tree root which can be recovered from
// the merkle proof embedded in the payload but itself is not
// stored the payload.
//
// Shred anatomy:
//
//	Legacy Data (1228b):
//	+---------------------+--------------------+-----------------+
//	| COMMON_HEADER (83b) | DATA HEADER (88b)  | PAYLOAD (1057b) |
//	+---------------------+--------------------+-----------------+
//	| [SIG, VARIANT, ...] | [FLAGS, SIZE, ...] | BINARY DATA     |
//	+---------------------+--------------------+-----------------+
//
//	Legacy Coding (1228b):
//	+---------------------+---------------------+-----------------+
//	| COMMON_HEADER (83b) | CODING HEADER (89b) | PAYLOAD (1057b) |
//	+---------------------+---------------------+-----------------+
//	| [SIG, VARIANT, ...] | [FLAGS, SIZE, ...]  | BINARY DATA     |
//	+---------------------+---------------------+-----------------+
//
//	Merkle Data (1203b):
//	+---------------------+--------------------+-----------------+
//	| COMMON_HEADER (83b) | DATA HEADER (88b)  | PAYLOAD (1032b) |
//	+---------------------+--------------------+-----------------+
//	| [SIG, VARIANT, ...] | [FLAGS, SIZE, ...] | BINARY DATA     |
//	+---------------------+--------------------+-----------------+
//
//	Legacy Coding (1228b):
//	+---------------------+---------------------+--------------------+
//	| COMMON_HEADER (83b) | CODING HEADER (89b) | PAYLOAD (1057b)    |
//	+---------------------+---------------------+--------------------+
//	| [SIG, VARIANT, ...] | [FLAGS, SIZE, ...]  | BIN + MERKLE PROOF |
//	+---------------------+---------------------+--------------------+
//
// see solana/ledger/src/shred.rs
const sizeOfCommonShredHeader = 83
const sizeOfSignature = 64
const sizeOfShredVariant = 1
const sizeOfShredSlot = 8
const sizeOfShredIndex = 4
const sizeOfVersion = 2
const sizeOfFecSet = 4
const sizeOfu16 = 2
const sizeOfLegacyDataShredPayload = 1228
const sizeOfLegacyCodeShredPayload = sizeOfLegacyDataShredPayload
const sizeOfMerkleCodeShredHeaders = 89
const sizeOfMerkleCodeShredPayload = 1228
const sizeOfMerkleDataShredPayload = sizeOfMerkleCodeShredPayload -
	sizeOfMerkleCodeShredHeaders + sizeOfSignature

const (
	ShredVariantLegacyDataByte = 0b1010 << 4
	ShredVariantLegacyCodeByte = 0b0101 << 4
	ShredVariantMerkleDataByte = 0b1000 << 4
	ShredVariantMerkleCodeByte = 0b0100 << 4
)

var shredVariantString = map[byte]string{
	ShredVariantLegacyDataByte: "LegacyData",
	ShredVariantLegacyCodeByte: "LegacyCode",
	ShredVariantMerkleDataByte: "MerkleData",
	ShredVariantMerkleCodeByte: "MerkleCode",
}

// ShredVariant describes first 4 bits of shred_variant byte from shred ShredCommonHeader
// we can only check first 4 bits because in Merkle shreds last 4 bits are used to
// specify the number of merkle proof entries:
// ledger/src/shred.rs
//
// LegacyCode 0b0101_1010
// LegacyData 0b1010_0101
// MerkleCode 0b0100_????
// MerkleData 0b1000_????
type ShredVariant byte

func (s ShredVariant) String() string {
	str, ok := shredVariantString[byte(s)]
	if !ok {
		return ""
	}

	return str
}

func (s ShredVariant) IsData() bool {
	return s == ShredVariantLegacyDataByte || s == ShredVariantMerkleDataByte
}

func (s ShredVariant) IsCode() bool {
	return s == ShredVariantLegacyCodeByte || s == ShredVariantMerkleCodeByte
}

type Shred struct {
	Raw []byte

	DataSize  int
	Signature string
	Variant   ShredVariant
	Slot      uint64
	Index     uint32
	FecSet    uint32

	CodingShredNumDataShreds uint16
	CodingShredNumCodeShreds uint16
	CodingShredPosition      uint16
}

// PartialShred has only information about shred type, slot and index, Raw is full shred
type PartialShred struct {
	Raw     []byte
	Variant ShredVariant
	Slot    uint64
	Index   uint32
}

func ParseShred(s []byte) (*Shred, error) {
	if err := validateShredSize(len(s)); err != nil {
		return nil, err
	}

	signatureBin := s[:sizeOfSignature]
	signature := base64.StdEncoding.EncodeToString(signatureBin)

	// A mask is a value used to enable, disable, or modify particular bits within another value.
	// This is typically done using bitwise operations, such as AND, OR, and XOR.
	// In our case, we're interested in isolating the first 4 bits of a byte and ignoring the last 4 bits.
	// To achieve this, we use a mask with the first 4 bits set to 1 and the last 4 bits set to 0.
	// This mask is represented as 0b11110000.
	shredVariant := ShredVariant(s[sizeOfSignature : sizeOfSignature+sizeOfShredVariant][0] & 0b11110000)

	var codingShredNumDataShreds uint16
	var codingShredNumCodeShreds uint16
	var codingShredPosition uint16
	if shredVariant.IsCode() {
		numDataShredsBin := s[sizeOfCommonShredHeader : sizeOfCommonShredHeader+sizeOfu16]
		codingShredNumDataShreds = binary.LittleEndian.Uint16(numDataShredsBin)

		numCodeShredsBin := s[sizeOfCommonShredHeader+sizeOfu16 : sizeOfCommonShredHeader+(sizeOfu16*2)]
		codingShredNumCodeShreds = binary.LittleEndian.Uint16(numCodeShredsBin)

		positionBin := s[sizeOfCommonShredHeader+(sizeOfu16*2) : sizeOfCommonShredHeader+(sizeOfu16*3)]
		codingShredPosition = binary.LittleEndian.Uint16(positionBin)
	}

	slotBin := s[sizeOfSignature+sizeOfShredVariant : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot]
	slot := binary.LittleEndian.Uint64(slotBin)

	indexBin := s[sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot+sizeOfShredIndex]
	index := binary.LittleEndian.Uint32(indexBin)

	fecSetBin := s[sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot+sizeOfShredIndex+sizeOfVersion : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot+sizeOfShredIndex+sizeOfVersion+sizeOfFecSet]
	fecSet := binary.LittleEndian.Uint32(fecSetBin)

	return &Shred{
		Raw:       s,
		DataSize:  len(s),
		Signature: signature,
		Variant:   shredVariant,
		Slot:      slot,
		Index:     index,
		FecSet:    fecSet,

		CodingShredNumDataShreds: codingShredNumDataShreds,
		CodingShredNumCodeShreds: codingShredNumCodeShreds,
		CodingShredPosition:      codingShredPosition,
	}, nil
}

func ParseShredPartial(s []byte) (*PartialShred, error) {
	if err := validateShredSize(len(s)); err != nil {
		return nil, err
	}

	return &PartialShred{
		Raw:     s,
		Variant: ShredVariant(s[sizeOfSignature : sizeOfSignature+sizeOfShredVariant][0] & 0b11110000),
		Slot: binary.LittleEndian.Uint64(
			s[sizeOfSignature+sizeOfShredVariant : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot]),
		Index: binary.LittleEndian.Uint32(
			s[sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot+sizeOfShredIndex]),
	}, nil
}

func ShredKey(slot uint64, index uint32, variant ShredVariant) string {
	var builder strings.Builder
	builder.WriteString(strconv.Itoa(int(slot)))
	builder.WriteString(strconv.Itoa(int(index)))
	// not calling variant.String() to avoid map access to improve conversion performance
	// this will essentially write unique non-ascii string to keep the diff between different
	// shred variants
	builder.WriteString(string(variant))
	return builder.String()
}

func validateShredSize(sz int) error {
	if sz != sizeOfLegacyCodeShredPayload && sz != sizeOfMerkleDataShredPayload {
		return fmt.Errorf("illegal shred size, expected: %d for legacy data/code & merkle code shreds or %d for merkle data shred, got: %d",
			sizeOfLegacyCodeShredPayload, sizeOfMerkleDataShredPayload, sz)
	}

	return nil
}
