// Code generated by "stringer -type=ResponseKind"; DO NOT EDIT.

package message

import "strconv"

const _ResponseKind_name = "KindStatusKindIntKindStringKindStringSlice"

var _ResponseKind_index = [...]uint8{0, 10, 17, 27, 42}

func (i ResponseKind) String() string {
	if i < 0 || i >= ResponseKind(len(_ResponseKind_index)-1) {
		return "ResponseKind(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _ResponseKind_name[_ResponseKind_index[i]:_ResponseKind_index[i+1]]
}