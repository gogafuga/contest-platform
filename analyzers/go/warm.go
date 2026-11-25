// Fixed build-time fixture: cache common standard-library packages only.
package main
import (
 _ "bufio"
 _ "bytes"
 _ "encoding/json"
 _ "fmt"
 _ "io"
 _ "math"
 _ "net/netip"
 _ "os"
 _ "regexp"
 _ "sort"
 _ "strconv"
 _ "strings"
 _ "unicode"
 _ "unicode/utf8"
)
func main() {}
