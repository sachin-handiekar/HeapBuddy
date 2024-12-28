# HeapBuddy

HeapBuddy is a command-line heap dump (HPROF) analyzer that helps you understand and optimize your Java applications' memory usage. It provides detailed insights into object allocation, memory leaks, and heap usage patterns.

## Features

- Parse and analyze Java HPROF heap dump files
- Memory leak detection
- Object reference analysis
- Heap composition overview
- Memory usage patterns
- HTML report generation

## Getting Started

### Prerequisites

- Go 1.21 or higher

### Installation

1. Clone the repository
2. Run `go mod download` to install dependencies
3. Build the application:
   ```bash
   # Windows
   go build -o heapbuddy.exe
   ```

## Usage

Analyze a heap dump file:
```bash
heapbuddy analyze <file_name>.hprof
```

This will generate an HTML report with detailed analysis of:
- Memory usage overview
- Class histogram
- Largest objects
- System properties

## License

MIT License
