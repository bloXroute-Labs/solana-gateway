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
	LegacyCode         byte = 0b0101 << 4
	LegacyData         byte = 0b1010 << 4
	MerkleCode         byte = 0b0100 << 4
	MerkleCodeChained  byte = 0b0110 << 4
	MerkleCodeResigned byte = 0b0111 << 4
	MerkleData         byte = 0b1000 << 4
	MerkleDataChained  byte = 0b1001 << 4
	MerkleDataResigned byte = 0b1011 << 4
)

const (
	// A mask is a value used to enable, disable, or modify particular bits within another value.
	// This is typically done using bitwise operations, such as AND, OR, and XOR.
	// In our case, we're interested in isolating the first 2 bits of a byte and ignoring the last 6 bits.
	// To achieve this, we use a mask with the first 2 bits set to 1 and the last 6 bits set to 0.
	// This mask is represented as 0b11000000.
	shredMask    = 0b11 << 6
	shredCodeCmp = 0b01 << 6
	shredDataCmp = 0b10 << 6
)

func (s *ShredVariantByte) IsCode() bool {
	return (s.Variant & byte(shredMask)) == shredCodeCmp
}

func (s *ShredVariantByte) IsData() bool {
	return (s.Variant & byte(shredMask)) == shredDataCmp
}

func (s *ShredVariantByte) String() string {
	return s.VariantString
}

// ShredVariantByte represents shred variant
type ShredVariantByte struct {
	Variant       byte
	VariantString string
	ProofSize     uint8
	Chained       bool
	Resigned      bool
}

// ParseShredVariant accepts a byte and returns a ShredVariantByte struct.
func ParseShredVariant(b byte) (*ShredVariantByte, error) {
	// extract the first 4 bits to identify the variant
	variant := b & 0b11110000
	// extract the last 4 bits to get the proof size
	proofSize := b & 0b00001111

	switch variant {
	case LegacyCode:
		return &ShredVariantByte{Variant: variant, VariantString: "LegacyCode"}, nil
	case LegacyData:
		return &ShredVariantByte{Variant: variant, VariantString: "LegacyData"}, nil
	case MerkleCode:
		return &ShredVariantByte{Variant: variant, VariantString: "MerkleCode", ProofSize: proofSize}, nil
	case MerkleCodeChained:
		return &ShredVariantByte{Variant: variant, VariantString: "MerkleCodeChained", ProofSize: proofSize, Chained: true}, nil
	case MerkleCodeResigned:
		return &ShredVariantByte{Variant: variant, VariantString: "MerkleCodeResigned", ProofSize: proofSize, Chained: true, Resigned: true}, nil
	case MerkleData:
		return &ShredVariantByte{Variant: variant, VariantString: "MerkleData", ProofSize: proofSize}, nil
	case MerkleDataChained:
		return &ShredVariantByte{Variant: variant, VariantString: "MerkleDataChained", ProofSize: proofSize, Chained: true}, nil
	case MerkleDataResigned:
		return &ShredVariantByte{Variant: variant, VariantString: "MerkleDataResigned", ProofSize: proofSize, Chained: true, Resigned: true}, nil
	default:
		return nil, fmt.Errorf("unknown shred variant: %08b", b)
	}
}

type Shred struct {
	Raw []byte

	DataSize  int
	Signature string
	Variant   *ShredVariantByte
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
	Variant *ShredVariantByte
	Slot    uint64
	Index   uint32
}

func ParseShred(s []byte) (*Shred, error) {
	if len(s) < sizeOfSignature {
		return nil, fmt.Errorf("shred size too small: %d", len(s))
	}

	shredVariant, err := ParseShredVariant(s[sizeOfSignature])
	if err != nil {
		return nil, fmt.Errorf("parse shred variant byte: %w", err)
	}

	if err := validateShredSize(len(s), shredVariant); err != nil {
		return nil, err
	}

	signatureBin := s[:sizeOfSignature]
	signature := base64.StdEncoding.EncodeToString(signatureBin)

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
	if len(s) < sizeOfSignature {
		return nil, fmt.Errorf("shred size too small: %d", len(s))
	}

	shredVariant, err := ParseShredVariant(s[sizeOfSignature])
	if err != nil {
		return nil, err
	}

	if err := validateShredSize(len(s), shredVariant); err != nil {
		return nil, err
	}

	return &PartialShred{
		Raw:     s,
		Variant: shredVariant,
		Slot: binary.LittleEndian.Uint64(
			s[sizeOfSignature+sizeOfShredVariant : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot]),
		Index: binary.LittleEndian.Uint32(
			s[sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot+sizeOfShredIndex]),
	}, nil
}

func ShredKey(slot uint64, index uint32, variant *ShredVariantByte) string {
	var builder strings.Builder
	builder.WriteString(strconv.Itoa(int(slot)))
	builder.WriteString(strconv.Itoa(int(index)))
	// not calling variant.String() to avoid map access to improve conversion performance
	// this will essentially write unique non-ascii char to keep the diff between different
	// shred variants
	builder.WriteString(string(variant.Variant))
	return builder.String()
}

func validateShredSize(sz int, shredVariant *ShredVariantByte) error {
	switch shredVariant.Variant {
	case LegacyCode, LegacyData:
		if sz != sizeOfLegacyCodeShredPayload {
			return fmt.Errorf("illegal shred size, expected: %d for legacy data/code, got: %d", sizeOfLegacyCodeShredPayload, sz)
		}
	case MerkleCode, MerkleData, MerkleDataChained, MerkleDataResigned, MerkleCodeResigned, MerkleCodeChained:
		if sz < sizeOfMerkleDataShredPayload || sz > sizeOfLegacyCodeShredPayload {
			return fmt.Errorf("illegal shred size, expected between %d to %d for merkle data/code, got: %d", sizeOfMerkleDataShredPayload, sizeOfLegacyCodeShredPayload, sz)
		}
	default:
		return fmt.Errorf("unknown shred variant: %d", shredVariant.Variant)
	}

	return nil
}
