package manta

import (
	"math"
	"strconv"
)

func decodeHandle(r *Reader, f *dt_field) interface{} {
	// So far these seem to occupy 32 bits but the value is made up only
	// out of what's present in the first 21 bits. In source 1, these only
	// occupied 21 bits of space.
	value := r.readBits(21) // a uint32
	r.seekBits(11)          // skip the rest of the 32 bits
	return value
}

func decodeByte(r *Reader, f *dt_field) interface{} {
	return r.readBits(8)
}

func decodeShort(r *Reader, f *dt_field) interface{} {
	return r.readBits(16)
}

func decodeUnsigned(r *Reader, f *dt_field) interface{} {
	return r.readVarUint64()
}

func decodeSigned(r *Reader, f *dt_field) interface{} {
	return r.readVarInt32()
}

func decodeSigned64(r *Reader, f *dt_field) interface{} {
	return r.readVarInt64()
}

func decodeBoolean(r *Reader, f *dt_field) interface{} {
	return r.readBoolean()
}

func decodeFloat(r *Reader, f *dt_field) interface{} {
	// Parse specific encoders
	switch f.Encoder {
	case "coord":
		return r.readCoord()
	}

	if f.BitCount != nil {
		// equivalent to the old noscale
		return decodeFloatNoscale(r, f)
	} else {
		return r.readVarUint32()
	}
}

func decodeFloatNoscale(r *Reader, f *dt_field) interface{} {
	return math.Float32frombits(r.readBits(int(*f.BitCount)))
}

func decodeQuantized(r *Reader, f *dt_field) interface{} {
	_debugf(
		"Quantized, Bitcount: %v, Low: %v, High: %v, Flags: %v, Encoder: %v",
		saveReturnInt32(f.BitCount),
		saveReturnFloat32(f.LowValue, "nil"),
		saveReturnFloat32(f.HighValue, "nil"),
		strconv.FormatInt(int64(saveReturnInt32(f.Flags)), 2),
		f.Encoder,
	)

	var BitCount int
	var Low float32
	var High float32
	var Range float32
	var Offset float32

	if f.BitCount != nil {
		BitCount = int(*f.BitCount)
	}

	if f.LowValue != nil {
		Low = *f.LowValue
	} else {
		Low = 0.0
	}

	if f.HighValue != nil {
		High = *f.HighValue
	} else {
		High = 1.0
	}

	// Verify mutualy exclusive flags
	if *f.Flags&(qf_rounddown|qf_roundup) == (qf_rounddown | qf_roundup) {
		_panicf("Roundup / Rounddown are mutually exclusive")
	}

	// Verify min / max
	if Low > High {
		_panicf("Inverted min / max values")
	}

	steps := (1 << uint(BitCount))

	// Set range and offset for roundup / rounddown
	if (*f.Flags & qf_rounddown) != 0 {
		Range = High - Low
		Offset = (Range / float32(steps))
		High -= Offset
	} else if (*f.Flags & qf_roundup) != 0 {
		Range = High - Low
		Offset = (Range / float32(steps))
		Low += Offset
	}

	// Handle integer encoding flag
	if (*f.Flags & qf_encode_integers) != 0 {
		delta := High - Low

		if delta < 1 {
			delta = 1
		}

		deltaLog2 := uint(math.Log2(float64(delta)) + 1)
		Range2 := (1 << deltaLog2)
		bc := BitCount

		for 1 == 1 {
			if (1 << uint(bc)) > Range2 {
				break
			} else {
				bc++
			}
		}

		if bc > BitCount {
			_debugf("Upping bitcount for qf_encode_integers field %v -> %v", BitCount, bc)
			BitCount = bc
			steps = (1 << uint(BitCount))
		}

		Offset = float32(Range2) / float32(steps)
		High = Low + float32(Range2) - Offset
	}

	if (*f.Flags & 0x100) != 0 {
		r.seekBits(int(*f.BitCount))
		return 0.0
	} else {
		if (*f.Flags&0x10) != 0 && r.readBoolean() {
			return Low
		}

		if (*f.Flags&0x20) != 0 && r.readBoolean() {
			return High
		}

		if (*f.Flags&0x40) != 0 && r.readBoolean() {
			return 0.0
		}

		intVal := r.readBits(BitCount)
		flVal := float32(intVal) * (1.0 / (float32(uint(1<<uint(BitCount))) - 1))
		flVal = Low + (High-Low)*flVal
		return flVal
	}
}

func decodeString(r *Reader, f *dt_field) interface{} {
	return r.readString()
}

func decodeVector(r *Reader, f *dt_field) interface{} {
	size := r.readVarUint32()

	if size > 0 {
		_panicf("Ive been called, %v", size)
	}

	return 0
}

func decodeClass(r *Reader, f *dt_field) interface{} {
	return r.readVarUint32()
}

func decodeFVector(r *Reader, f *dt_field) interface{} {
	// Parse specific encoders
	switch f.Encoder {
	case "normal":
		return r.read3BitNormal()
	}

	return []float32{decodeFloat(r, f).(float32), decodeFloat(r, f).(float32), decodeFloat(r, f).(float32)}
}

func decodeNop(r *Reader, f *dt_field) interface{} {
	return 0
}

func decodePointer(r *Reader, f *dt_field) interface{} {
	// Seems to be encoded as a single bit, not sure what to make of it
	if !r.readBoolean() {
		_panicf("Figure out how this works")
	}

	return 0
}

func decodeQAngle(r *Reader, f *dt_field) interface{} {
	ret := [3]float32{0.0, 0.0, 0.0}

	// Parse specific encoders
	switch f.Encoder {
	case "qangle_pitch_yaw":
		if f.BitCount != nil && f.Flags != nil && (*f.Flags&0x20 != 0) {
			_panicf("Special Case: Unkown for now")
		}

		ret[0] = r.readAngle(uint(*f.BitCount))
		ret[1] = r.readAngle(uint(*f.BitCount))
		return ret
	}

	// Parse a standard angle
	if f.BitCount != nil && *f.BitCount == 32 {
		_panicf("Special Case: Unkown for now")
	} else if f.BitCount != nil && *f.BitCount != 0 {
		ret[0] = r.readAngle(uint(*f.BitCount))
		ret[1] = r.readAngle(uint(*f.BitCount))
		ret[2] = r.readAngle(uint(*f.BitCount))

		return ret
	} else {
		rX := r.readBoolean()
		rY := r.readBoolean()
		rZ := r.readBoolean()

		if rX {
			ret[0] = r.readCoord()
		}

		if rY {
			ret[1] = r.readCoord()
		}

		if rZ {
			ret[2] = r.readCoord()
		}

		return ret
	}

	_panicf("No valid encoding determined")
	return ret
}

func decodeComponent(r *Reader, f *dt_field) interface{} {
	_debugf(
		"Bitcount: %v, Low: %v, High: %v, Flags: %v",
		saveReturnInt32(f.BitCount),
		saveReturnFloat32(f.LowValue, "nil"),
		saveReturnFloat32(f.HighValue, "nil"),
		strconv.FormatInt(int64(saveReturnInt32(f.Flags)), 2),
	)

	return r.readBits(1)
}

func decodeHSequence(r *Reader, f *dt_field) interface{} {
	// wrong, just testing
	return r.readBits(1)
}
