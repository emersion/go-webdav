package caldav_test

import (
	"testing"

	"github.com/emersion/go-webdav/caldav"
)

func TestCalendarCompRequest_IsEmpty(b *testing.T) {
	testCases := []struct {
		Name           string
		Request        caldav.CalendarCompRequest
		ExpectedResult bool
	}{
		{
			Name:           "empty",
			Request:        caldav.CalendarCompRequest{},
			ExpectedResult: true,
		},
		{
			Name: "has-name",
			Request: caldav.CalendarCompRequest{
				Name: "name",
			},
			ExpectedResult: false,
		},
	}

	for _, tCase := range testCases {
		b.Run(tCase.Name, func(t *testing.T) {
			if got, want := tCase.Request.Name == "", tCase.ExpectedResult; got != want { //nolint:scopelint
				t.Errorf("bad result: %t, expected: %t", got, want)
			}
		})
	}
}
