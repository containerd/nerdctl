package buildkitutil

import (
	"reflect"
	"testing"
)

func TestParseBuildctlPruneOutput(t *testing.T) {
	type args struct {
		out []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *BuildctlPruneOutput
		wantErr bool
	}{
		{
			name: "builtctl prune frees several spaces",
			args: args{out: []byte(`ID                                                                      RECLAIMABLE     SIZE    LAST ACCESSED
spr33ail5pfddqvf25mnxg013                                               true            0B
akklguozvrppljhuw3v9t45se*                                              true            4.10kB
skr3tplsx990ttmvyymww9pf2*                                              true            0B
hevhw67hx6yv6zlkob00lkaxm                                               true            6.84MB
lqq6c21uz1e697mgvah93govi                                               true            8.19kB
3qi6zgbbup6h6nuc2axyx39ed                                               true            140.81MB
ise2an3ziszpxqfbm0iwiow6p                                               true            327.58MB
0st72zjm0ei4r60hnbmr0thye                                               true            109.07MB
Total:  584.31MB`)},
			want: &BuildctlPruneOutput{
				TotalSize: "584.31MB",
				Rows: []BuildctlPruneOutputRow{
					{
						ID:           "spr33ail5pfddqvf25mnxg013",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "akklguozvrppljhuw3v9t45se*",
						Reclaimable:  "true",
						Size:         "4.10kB",
						LastAccessed: "",
					},
					{
						ID:           "skr3tplsx990ttmvyymww9pf2*",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "hevhw67hx6yv6zlkob00lkaxm",
						Reclaimable:  "true",
						Size:         "6.84MB",
						LastAccessed: "",
					},
					{
						ID:           "lqq6c21uz1e697mgvah93govi",
						Reclaimable:  "true",
						Size:         "8.19kB",
						LastAccessed: "",
					},
					{
						ID:           "3qi6zgbbup6h6nuc2axyx39ed",
						Reclaimable:  "true",
						Size:         "140.81MB",
						LastAccessed: "",
					},
					{
						ID:           "ise2an3ziszpxqfbm0iwiow6p",
						Reclaimable:  "true",
						Size:         "327.58MB",
						LastAccessed: "",
					},
					{
						ID:           "0st72zjm0ei4r60hnbmr0thye",
						Reclaimable:  "true",
						Size:         "109.07MB",
						LastAccessed: "",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "buildctl prune frees no spaces",
			args: args{
				out: []byte(`Total:  0B`),
			},
			want: &BuildctlPruneOutput{
				TotalSize: "0B",
				Rows:      nil,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBuildctlPruneTableOutput(tt.args.out)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBuildctlPruneTableOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseBuildctlPruneTableOutput() got = %v, want %v", got, tt.want)
			}
		})
	}
}
