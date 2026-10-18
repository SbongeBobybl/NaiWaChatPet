module codexpet/reconstruction

go 1.27

require (
 github.com/Microsoft/go-winio v0.6.2
 golang.org/x/sys v0.10.0
)

replace github.com/Microsoft/go-winio => ../third_party/go-winio-0.6.2
replace golang.org/x/sys => ../third_party/sys-0.10.0
