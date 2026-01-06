package solana

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	UDPShredSize = 1228
)

// Legacy data shreds are always zero padded and
// the same size as coding shreds.
//
// Merkle shreds sign merkle tree root which can be recovered from
// the merkle proof embedded in the payload but itself is not
// stored the payload.
//
// see solana/ledger/src/shred.rs
const (
	sizeOfCommonShredHeader      = 83
	sizeOfSignature              = 64
	sizeOfParent                 = 85
	sizeOfShredVariant           = 1
	sizeOfShredSlot              = 8
	sizeOfShredIndex             = 4
	sizeOfVersion                = 2
	sizeOfFecSet                 = 4
	sizeOfu16                    = 2
	sizeOfLegacyDataShredPayload = 1228
	sizeOfLegacyCodeShredPayload = sizeOfLegacyDataShredPayload
	sizeOfMerkleCodeShredHeaders = 89
	sizeOfMerkleCodeShredPayload = 1228
	sizeOfMerkleDataShredPayload = sizeOfMerkleCodeShredPayload -
		sizeOfMerkleCodeShredHeaders + sizeOfSignature
)

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

// see solana/ledger/src/blockstore.rs - MAX_DATA_SHREDS_PER_SLOT
const (
	maxDataShredsPerSlot = 32768
)

var (
	ErrShredTooSmall = errors.New("shred size too small")
	ErrShredTooBig   = errors.New("shred size too big")
)

var AliveMsg = []byte("alive")

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
func ParseShredVariant(b byte) (ShredVariantByte, error) {
	// extract the first 4 bits to identify the variant
	variant := b & 0b11110000
	// extract the last 4 bits to get the proof size
	proofSize := b & 0b00001111

	switch variant {
	case LegacyCode:
		return ShredVariantByte{Variant: variant, VariantString: "LegacyCode"}, nil
	case LegacyData:
		return ShredVariantByte{Variant: variant, VariantString: "LegacyData"}, nil
	case MerkleCode:
		return ShredVariantByte{Variant: variant, VariantString: "MerkleCode", ProofSize: proofSize}, nil
	case MerkleCodeChained:
		return ShredVariantByte{Variant: variant, VariantString: "MerkleCodeChained", ProofSize: proofSize, Chained: true}, nil
	case MerkleCodeResigned:
		return ShredVariantByte{Variant: variant, VariantString: "MerkleCodeResigned", ProofSize: proofSize, Chained: true, Resigned: true}, nil
	case MerkleData:
		return ShredVariantByte{Variant: variant, VariantString: "MerkleData", ProofSize: proofSize}, nil
	case MerkleDataChained:
		return ShredVariantByte{Variant: variant, VariantString: "MerkleDataChained", ProofSize: proofSize, Chained: true}, nil
	case MerkleDataResigned:
		return ShredVariantByte{Variant: variant, VariantString: "MerkleDataResigned", ProofSize: proofSize, Chained: true, Resigned: true}, nil
	default:
		return ShredVariantByte{}, fmt.Errorf("unknown shred variant: %08b", b)
	}
}

type Shred struct {
	Raw []byte

	DataSize  int
	Signature string
	Variant   ShredVariantByte
	Slot      uint64
	Index     uint32
	FecSet    uint32

	CodingShredNumDataShreds uint16
	CodingShredNumCodeShreds uint16
	CodingShredPosition      uint16
}

// PartialShred has only information about shred type, slot and index, Raw is full shred
type PartialShred struct {
	Raw      [1228]byte
	DataSize int
	Variant  ShredVariantByte
	Slot     uint64
	Index    uint32
}

func ParseShred(s []byte) (Shred, error) {
	if len(s) < sizeOfSignature {
		return Shred{}, fmt.Errorf("shred size too small: %d", len(s))
	}

	shredVariant, err := ParseShredVariant(s[sizeOfSignature])
	if err != nil {
		return Shred{}, fmt.Errorf("parse shred variant byte: %w", err)
	}

	if err := validateShredSize(len(s), shredVariant); err != nil {
		return Shred{}, err
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

	return Shred{
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

func ParseShredPartial(s [1228]byte, size int) (PartialShred, error) {
	if size < sizeOfParent {
		return PartialShred{}, fmt.Errorf("shred size too small: %d", len(s))
	}

	shredVariant, err := ParseShredVariant(s[sizeOfSignature])
	if err != nil {
		return PartialShred{}, err
	}

	if err := validateShredSize(size, shredVariant); err != nil {
		return PartialShred{}, err
	}
	index := binary.LittleEndian.Uint32(
		s[sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot+sizeOfShredIndex])

	if index > maxDataShredsPerSlot {
		return PartialShred{}, fmt.Errorf("shred index is too big: %d", index)
	}

	slot := binary.LittleEndian.Uint64(
		s[sizeOfSignature+sizeOfShredVariant : sizeOfSignature+sizeOfShredVariant+sizeOfShredSlot])

	if shredVariant.IsData() {
		parentOffset := binary.LittleEndian.Uint16(s[sizeOfCommonShredHeader:sizeOfParent])
		if slot < uint64(parentOffset) {
			return PartialShred{}, fmt.Errorf("shred slot %d is smaller than parent_offset %d", slot, parentOffset)
		}
		actualParentSlot := slot - uint64(parentOffset)
		if slot < actualParentSlot {
			return PartialShred{}, fmt.Errorf("shred slot %v is smaller than parent %v", slot, actualParentSlot)
		}
	}

	return PartialShred{
		Raw:      s,
		DataSize: size,
		Variant:  shredVariant,
		Slot:     slot,
		Index:    index,
	}, nil
}

func ShredKey(slot uint64, index uint32, variant ShredVariantByte) uint64 {
	// slot: 36 bits (positions 28-63) - supports up to 68,719,476,735
	//     assuming that 1 slot will stay for 400ms, it have maximum capacity of 800 years
	// index: 20 bits (positions 8-27) - supports up to 1,048,575
	// variant: 8 bits (positions 0-7) - supports up to 255
	return (slot << 28) | (uint64(index) << 8) | uint64(variant.Variant)
}

func validateShredSize(sz int, shredVariant ShredVariantByte) error {
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

func ValidateShredSizeSimple(size int) error {
	if size > sizeOfLegacyCodeShredPayload {
		return ErrShredTooBig
	}

	if size < sizeOfMerkleDataShredPayload {
		return ErrShredTooSmall
	}

	return nil
}

func ShouldSample(shred PartialShred) bool {
	// Every ~30sec
	return shred.Slot%75 == 0 && shred.Index == 100
}
