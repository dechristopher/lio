package clock

import (
	"testing"
	"time"
)

func Test_playerClock_takeTime(t *testing.T) {
	type fields struct {
		control TimeControl
		elapsed CTime
	}

	type args struct {
		t CTime
	}

	const errFmt = "time taken, expected: %s, got: %s"

	tests := []struct {
		name     string
		fields   fields
		args     args
		expected CTime
	}{
		{
			name: "take time 1s",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args:     args{t: ToCTime(time.Second * 1)},
			expected: ToCTime(time.Second * 59),
		},
		{
			name: "take time 10s",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args:     args{t: ToCTime(time.Second * 10)},
			expected: ToCTime(time.Second * 50),
		},
		{
			name: "take time into negative",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args:     args{t: ToCTime(time.Second * 61)},
			expected: ToCTime(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &playerClock{
				control: tt.fields.control,
				elapsed: tt.fields.elapsed,
			}
			pc.giveTime(tt.args.t)

			if pc.remaining() != tt.expected {
				t.Errorf(errFmt, tt.expected.String(), pc.remaining().String())
			}
		})
	}
}

func Test_playerClock_giveTime(t *testing.T) {
	type fields struct {
		control TimeControl
		elapsed CTime
	}

	type args struct {
		t CTime
	}

	const errFmt = "time given, expected: %s, got: %s"

	tests := []struct {
		name     string
		fields   fields
		args     args
		expected CTime
	}{
		{
			name: "give time 1s",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args:     args{t: ToCTime(time.Second * 1)},
			expected: ToCTime(time.Second * 61),
		},
		{
			name: "give time 10s",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 20),
				},
				elapsed: CTime{},
			},
			args:     args{t: ToCTime(time.Second * 10)},
			expected: ToCTime(time.Second * 30),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &playerClock{
				control: tt.fields.control,
				elapsed: tt.fields.elapsed,
			}
			pc.takeTime(tt.args.t)

			if pc.remaining() != tt.expected {
				t.Errorf(errFmt, tt.expected.String(), pc.remaining().String())
			}
		})
	}
}

func Test_playerClock_flagged(t *testing.T) {
	type fields struct {
		control TimeControl
		elapsed CTime
	}

	type args struct {
		t CTime
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "1s remaining, no flag",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args: args{t: ToCTime(time.Second * 59)},
			want: false,
		},
		{
			name: "40s remaining, no flag",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args: args{t: ToCTime(time.Second * 20)},
			want: false,
		},
		{
			name: "no time remaining, flag",
			fields: fields{
				control: TimeControl{
					Time: ToCTime(time.Second * 60),
				},
				elapsed: CTime{},
			},
			args: args{t: ToCTime(time.Second * 61)},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &playerClock{
				control: tt.fields.control,
				elapsed: tt.fields.elapsed,
			}

			pc.giveTime(tt.args.t)

			if got := pc.flagged(); got != tt.want {
				t.Errorf("flagged() = %v, want %v", got, tt.want)
			}
		})
	}
}
