package bencode

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode"
)

func DecodeBencode(data []byte) (interface{}, int, error) {

	// STRING
// STRING
if unicode.IsDigit(rune(data[0])) {
    colon := bytes.IndexByte(data, ':')
    length, err := strconv.Atoi(string(data[:colon]))
    if err != nil {
        return nil, 0, err
    }

    start := colon + 1
    end := start + length
    consumed := colon + 1 + length  // ← FIX: must include colon position

    return data[start:end], consumed, nil
}
	// INTEGER
	if data[0] == 'i' {

		end := bytes.IndexByte(data, 'e')

		num, err := strconv.Atoi(string(data[1:end]))

		return num, end + 1, err
	}

	// LIST
	if data[0] == 'l' {

		var list []interface{}

		i := 1

		for i < len(data) && data[i] != 'e' {

			value, consumed, err := DecodeBencode(data[i:])
			if err != nil {
				return nil, 0, err
			}

			list = append(list, value)

			i += consumed
		}

		return list, i + 1, nil
	}

	// DICTIONARY
	if data[0] == 'd' {

		dict := make(map[string]interface{})

		i := 1

		for i < len(data) && data[i] != 'e' {

			keyRaw, consumedKey, err := DecodeBencode(data[i:])
			if err != nil {
				return nil, 0, err
			}

			key := string(keyRaw.([]byte))

			i += consumedKey

			value, consumedValue, err := DecodeBencode(data[i:])
			if err != nil {
				return nil, 0, err
			}

			dict[key] = value

			i += consumedValue
		}

		return dict, i + 1, nil
	}

	return nil, 0, fmt.Errorf("unsupported bencode type: %q", data[0])
}
