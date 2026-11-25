// Offline fallback for known standard-library receivers. Semantic/user-defined
// members still come from the isolated language server; no code is evaluated.
const members: Record<string, Record<string, string>> = {
  go: {
    fmt: 'Print Printf Println Fprint Fprintf Fprintln Sprint Sprintf Sprintln Errorf Scan Scanf Scanln Fscan Fscanf Fscanln',
    os: 'ReadFile WriteFile Open OpenFile Create Stdin Stdout Stderr Args Exit Getenv LookupEnv Stat Remove MkdirAll',
    bufio: 'NewScanner NewReader NewWriter ScanLines ScanWords',
    strings: 'Split SplitN Fields TrimSpace Trim TrimPrefix TrimSuffix HasPrefix HasSuffix Contains Join ReplaceAll ToLower ToUpper Count Cut Index Builder NewReader',
    strconv: 'Atoi Itoa ParseInt ParseUint ParseFloat FormatInt FormatFloat Quote',
    sort: 'Strings Ints Slice SliceStable Search',
    slices: 'Sort SortFunc Contains Index Reverse Compact',
    bytes: 'Split Fields TrimSpace Contains Equal NewBuffer NewReader',
    math: 'Abs Min Max Pow Sqrt Floor Ceil Round',
    regexp: 'Compile MustCompile MatchString QuoteMeta',
    io: 'ReadAll Copy WriteString EOF',
    netip: 'ParseAddr MustParseAddr ParsePrefix MustParsePrefix',
  },
  python: {
    os: 'path environ getenv listdir mkdir makedirs remove rename getcwd chdir',
    'os.path': 'basename dirname join exists isfile isdir split splitext abspath normpath',
    sys: 'stdin stdout stderr argv exit',
    re: 'compile match fullmatch search findall finditer split sub escape IGNORECASE',
    json: 'load loads dump dumps',
    math: 'sqrt floor ceil pow log gcd isqrt inf pi',
    collections: 'Counter defaultdict deque',
    itertools: 'chain combinations permutations product groupby accumulate',
    heapq: 'heappush heappop heapify nlargest nsmallest',
    bisect: 'bisect_left bisect_right insort_left insort_right',
    pathlib: 'Path',
  },
  csharp: {
    Console: 'Write WriteLine Read ReadLine Error In Out',
    'System.Console': 'Write WriteLine Read ReadLine Error In Out',
    File: 'ReadAllText ReadAllLines ReadLines WriteAllText WriteAllLines AppendAllText AppendAllLines OpenRead OpenWrite Create CreateText Exists Delete',
    'System.IO.File': 'ReadAllText ReadAllLines ReadLines WriteAllText WriteAllLines AppendAllText AppendAllLines OpenRead OpenWrite Create CreateText Exists Delete',
    Directory: 'Exists CreateDirectory GetFiles GetDirectories EnumerateFiles Delete',
    Path: 'Combine GetFileName GetDirectoryName GetExtension GetFullPath',
    Math: 'Abs Min Max Pow Sqrt Floor Ceiling Round',
    Convert: 'ToInt32 ToInt64 ToDouble ToString',
    Array: 'Sort Reverse IndexOf BinarySearch Empty',
    Regex: 'IsMatch Match Matches Split Replace Escape',
  },
}
export interface StandardMember { label: string; receiver: string }
export function standardMembers(language: string, beforeCursor: string): StandardMember[] {
  const match = /([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*)\.([A-Za-z_]\w*)?$/.exec(beforeCursor)
  if (!match) return []
  const receiver = match[1]
  return (members[language]?.[receiver]?.split(' ') ?? []).map(label => ({ label, receiver }))
}
