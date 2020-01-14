package internal

import (
	"encoding/xml"
	"io"
)

// RawXMLValue is a raw XML value. It implements xml.Unmarshaler and
// xml.Marshaler and can be used to delay XML decoding or precompute an XML
// encoding.
type RawXMLValue struct {
	tok      xml.Token // guaranteed not to be xml.EndElement
	children []RawXMLValue
}

// UnmarshalXML implements xml.Unmarshaler.
func (val *RawXMLValue) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	val.tok = start
	val.children = nil

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			child := RawXMLValue{}
			if err := child.UnmarshalXML(d, tok); err != nil {
				return err
			}
			val.children = append(val.children, child)
		case xml.EndElement:
			return nil
		default:
			val.children = append(val.children, RawXMLValue{tok: xml.CopyToken(tok)})
		}
	}
}

// MarshalXML implements xml.Marshaler.
func (val *RawXMLValue) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	switch tok := val.tok.(type) {
	case xml.StartElement:
		if err := e.EncodeToken(tok); err != nil {
			return err
		}
		for _, child := range val.children {
			// TODO: find a sensible value for the start argument?
			if err := child.MarshalXML(e, xml.StartElement{}); err != nil {
				return err
			}
		}
		return e.EncodeToken(tok.End())
	case xml.EndElement:
		panic("unexpected end element")
	default:
		return e.EncodeToken(tok)
	}
}

var _ xml.Marshaler = (*RawXMLValue)(nil)
var _ xml.Unmarshaler = (*RawXMLValue)(nil)

// TokenReader returns a stream of tokens for the XML value.
func (val *RawXMLValue) TokenReader() xml.TokenReader {
	return &rawXMLValueReader{val: val}
}

type rawXMLValueReader struct {
	val         *RawXMLValue
	start, end  bool
	child       int
	childReader xml.TokenReader
}

func (tr *rawXMLValueReader) Token() (xml.Token, error) {
	if tr.end {
		return nil, io.EOF
	}

	start, ok := tr.val.tok.(xml.StartElement)
	if !ok {
		tr.end = true
		return tr.val.tok, nil
	}

	if !tr.start {
		tr.start = true
		return start, nil
	}

	for tr.child < len(tr.val.children) {
		if tr.childReader == nil {
			tr.childReader = tr.val.children[tr.child].TokenReader()
		}

		tok, err := tr.childReader.Token()
		if err == io.EOF {
			tr.childReader = nil
			tr.child++
		} else {
			return tok, err
		}
	}

	tr.end = true
	return start.End(), nil
}
