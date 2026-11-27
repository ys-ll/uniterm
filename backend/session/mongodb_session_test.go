package session

import "testing"

func TestBuildMongoURI(t *testing.T) {
	cases := []struct {
		name   string
		config ConnectionConfig
		want   string
	}{
		{
			name:   "defaults get authSource and directConnection",
			config: ConnectionConfig{Host: "10.0.0.1", Port: 27017},
			want:   "mongodb://10.0.0.1:27017?authSource=admin&directConnection=true",
		},
		{
			name:   "credentials are percent-escaped",
			config: ConnectionConfig{Host: "10.0.0.1", Port: 27017, User: "u@er", Password: "p/a#ss:word"},
			want:   "mongodb://u%40er:p%2Fa%23ss%3Aword@10.0.0.1:27017?authSource=admin&directConnection=true",
		},
		{
			name:   "dbname lands in path",
			config: ConnectionConfig{DBName: "mydb"},
			want:   "mongodb://127.0.0.1:27017/mydb?authSource=admin&directConnection=true",
		},
		{
			name:   "user authSource overrides the admin default without duplication",
			config: ConnectionConfig{DBParams: "authSource=mydb"},
			want:   "mongodb://127.0.0.1:27017?authSource=mydb&directConnection=true",
		},
		{
			name:   "replicaSet disables the directConnection default",
			config: ConnectionConfig{DBParams: "replicaSet=rs0"},
			want:   "mongodb://127.0.0.1:27017?authSource=admin&replicaSet=rs0",
		},
		{
			name:   "explicit directConnection is preserved",
			config: ConnectionConfig{DBParams: "directConnection=false"},
			want:   "mongodb://127.0.0.1:27017?authSource=admin&directConnection=false",
		},
		{
			name:   "extra params are merged alongside the defaults",
			config: ConnectionConfig{DBParams: "ssl=true&connectTimeoutMS=5000"},
			want:   "mongodb://127.0.0.1:27017?authSource=admin&connectTimeoutMS=5000&directConnection=true&ssl=true",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildMongoURI(tc.config)
			if got != tc.want {
				t.Errorf("buildMongoURI(%+v)\n  got:  %s\n  want: %s", tc.config, got, tc.want)
			}
		})
	}
}
