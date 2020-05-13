package bugzilla

import (
	"fmt"
	"time"
)

/*
source from buglist.cgi:

sub DiffDate {
  my ($datestr) = @_;
  my $date = str2time($datestr);
  my $age = time() - $date;

  if( $age < 18*60*60 ) {
      $date = format_time($datestr, '%H:%M:%S');
  } elsif( $age < 6*24*60*60 ) {
      $date = format_time($datestr, '%a %H:%M');
  } else {
      $date = format_time($datestr, '%Y-%m-%d');
  }
  return $date;
}
*/

type bugzillaChangedDateParser interface {
	utcNow() (t time.Time)
	parse(value string) (t time.Time, err error)
}
type baseParser struct{}
type defaultParser struct {
	baseParser
}
type timeParser struct {
	baseParser
}
type dayOfWeekParser struct {
	baseParser
}
type yearMonthDateParser struct {
	baseParser
}
type combinedParser struct {
	baseParser
}

type csvDateParser struct {
	baseParser
}

func (parser *baseParser) utcNow() (t time.Time) {
	return time.Now().UTC()
}

func (parser *combinedParser) parse(value string) (t time.Time, err error) {
	parsers := []bugzillaChangedDateParser{&defaultParser{}, &timeParser{}, &dayOfWeekParser{}, &yearMonthDateParser{},
		&csvDateParser{}}
	var lastErr error
	for _, item := range parsers {
		t, err := item.parse(value)
		if err != nil {
			lastErr = err
			continue
		} else {
			return t, nil
		}
	}
	return time.Time{}, lastErr
}

func (parser *csvDateParser) parse(value string) (t time.Time, err error) {
	format := "2006-01-02 15:04:05"
	return time.Parse(format, value)
}

func (parser *defaultParser) parse(value string) (t time.Time, err error) {
	format := "2006-01-02T15:04:05Z"
	return time.Parse(format, value)
}

func (parser *timeParser) parse(value string) (t time.Time, err error) {
	format := "15:04:05"
	parsedTime, err := time.Parse(format, value)
	if err != nil {
		return time.Time{}, err
	}
	now := parser.utcNow()
	t = time.Date(now.Year(), now.Month(), now.Day(), parsedTime.Hour(), parsedTime.Minute(), parsedTime.Second(), 0, time.UTC)
	if t.After(now) {
		t.AddDate(0, 0, -1)
	}
	return t, nil
}

func (parser *dayOfWeekParser) parse(value string) (t time.Time, err error) {
	format := "Mon 15:04"
	parsedTime, err := time.Parse(format, value)
	if err != nil {
		return time.Time{}, err
	}
	now := parser.utcNow()
	for i := 0; i <= 7; i++ {
		t = time.Date(now.Year(), now.Month(), now.Day(), parsedTime.Hour(), parsedTime.Minute(), 0, 0, time.UTC)
		if i != 0 {
			t = t.AddDate(0, 0, -1*i)
		}
		if int(parsedTime.Weekday()) == int(t.Weekday()) {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse %v", value)
}

func (parser *yearMonthDateParser) parse(value string) (t time.Time, err error) {
	format := "2006-01-02"
	parsedTime, err := time.Parse(format, value)
	if err != nil {
		return time.Time{}, err
	}
	t = time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, time.UTC)
	return t, nil
}
