// Code generated by "stringer -type=Provider"; DO NOT EDIT

package db

import "fmt"

const _Provider_name = "AmazonSpot"

var _Provider_index = [...]uint8{0, 10}

func (i Provider) String() string {
	if i < 0 || i >= Provider(len(_Provider_index)-1) {
		return fmt.Sprintf("Provider(%d)", i)
	}
	return _Provider_name[_Provider_index[i]:_Provider_index[i+1]]
}