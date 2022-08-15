package player

import (
	"reflect"
	"testing"

	"github.com/dechristopher/octad"
)

func TestAgreement_Agree(t *testing.T) {
	type args struct {
		color octad.Color
	}
	tests := []struct {
		name string
		a    Agreement
		args args
	}{
		{
			name: "test agree white",
			a:    make(Agreement),
			args: args{color: octad.White},
		},
		{
			name: "test agree black",
			a:    make(Agreement),
			args: args{color: octad.Black},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.a.Agree(tt.args.color)
			if !tt.a[tt.args.color] {
				t.Errorf("Agree(%s), a[%s] = %v, want %v", tt.args.color.String(), tt.args.color.String(), tt.a[tt.args.color], true)
			}
		})
	}
}

func TestAgreement_agreed(t *testing.T) {
	type args struct {
		color octad.Color
		agree bool
	}
	tests := []struct {
		name string
		a    Agreement
		args args
		want bool
	}{
		{
			name: "test agreed white",
			a:    make(Agreement),
			args: args{color: octad.White, agree: true},
			want: true,
		},
		{
			name: "test agreed black",
			a:    make(Agreement),
			args: args{color: octad.Black, agree: true},
			want: true,
		},
		{
			name: "test not agreed white",
			a:    make(Agreement),
			args: args{color: octad.White, agree: false},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.agree {
				tt.a.Agree(tt.args.color)
			}
			if got := tt.a.agreed(tt.args.color); got != tt.want {
				t.Errorf("agreed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgreement_Agreed(t *testing.T) {
	type args struct {
		agreed []octad.Color
	}
	tests := []struct {
		name string
		a    Agreement
		args args
		want bool
	}{
		{
			name: "test agreed neither",
			a:    make(Agreement),
			args: args{agreed: []octad.Color{}},
			want: false,
		},
		{
			name: "test agreed white",
			a:    make(Agreement),
			args: args{agreed: []octad.Color{octad.White}},
			want: false,
		},
		{
			name: "test agreed black",
			a:    make(Agreement),
			args: args{agreed: []octad.Color{octad.Black}},
			want: false,
		},
		{
			name: "test agreed both",
			a:    make(Agreement),
			args: args{agreed: []octad.Color{octad.White, octad.Black}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, color := range tt.args.agreed {
				tt.a.Agree(color)
			}

			if got := tt.a.Agreed(); got != tt.want {
				t.Errorf("Agreed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAgreement(t *testing.T) {
	tests := []struct {
		name string
		want Agreement
	}{
		{
			name: "new",
			want: NewAgreement(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAgreement(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAgreement() = %v, want %v", got, tt.want)
			}
		})
	}
}
