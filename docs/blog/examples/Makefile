# Build + run the leaking examples to produce .hprof dumps for the blog series.
#
#   make StaticCollectionLeak.hprof   # one dump
#   make all                          # every dump
#   make clean                        # remove *.class and *.hprof
#
# The OOM examples write a dump on death via -XX:+HeapDumpOnOutOfMemoryError.
# The "steady-state" examples (DuplicateStringWaste, OversizedCollections) run
# until you dump them with jcmd/jmap, so they're not in `all` — see README.

JAVAC ?= javac
JAVA  ?= java
HEAP  ?= 128m

OOM_EXAMPLES := StaticCollectionLeak ThreadLocalLeak ListenerLeak
OOM_DUMPS    := $(addsuffix .hprof,$(OOM_EXAMPLES))

.PHONY: all clean
all: $(OOM_DUMPS)

# Compile <Name>.java -> <Name>.class
%.class: %.java
	$(JAVAC) $<

# Run <Name> under a small heap; it OOMs and writes <Name>.hprof.
# `-` ignores the non-zero exit from the expected OutOfMemoryError.
%.hprof: %.class
	-$(JAVA) -Xmx$(HEAP) -XX:+HeapDumpOnOutOfMemoryError \
	        -XX:HeapDumpPath=$@ $*

clean:
	rm -f *.class *.hprof
