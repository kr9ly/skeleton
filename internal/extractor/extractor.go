package extractor

import "github.com/kr9ly/skeleton/skeleton"

type Extractor interface {
	Extract(src []byte) (*skeleton.File, error)
}
