/*
 * DuplicateStringWaste — wasted memory, not a leak.
 *
 * THE BUG: creating millions of String objects with the same content. Each is a
 * distinct object with its own backing char[]/byte[], so identical values cost
 * memory over and over. Nothing is "leaking" — the program just holds far more
 * bytes than the information requires.
 *
 * THE FIX: intern/canonicalize repeated values (String.intern(), an enum, a
 * shared constant, or a dedup map), or enable JVM string deduplication
 * (-XX:+UseStringDeduplication with G1).
 *
 * USED BY: blog #7 (duplicate strings & wasted memory).
 *
 * RUN (this one holds, rather than OOMs, so you can dump a steady state):
 *   javac DuplicateStringWaste.java
 *   java -Xmx512m DuplicateStringWaste &
 *   jcmd <pid> GC.heap_dump DuplicateStringWaste.hprof   # or jmap
 *
 * IN THE DUMP: HeapBuddy's "Duplicates & Wasted" section shows one enormous
 * duplicate-string group ("status=ACTIVE", etc.) accounting for most of the
 * heap — reclaimable memory, quantified.
 */
import java.util.ArrayList;
import java.util.List;

public class DuplicateStringWaste {

    public static void main(String[] args) throws InterruptedException {
        List<String> rows = new ArrayList<>();
        // new String(...) defeats the compile-time constant pool on purpose,
        // so every entry is a fresh, identical-but-distinct object.
        for (int i = 0; i < 5_000_000; i++) {
            rows.add(new String("status=ACTIVE"));
        }
        System.out.println("Holding " + rows.size() + " duplicate strings. "
                + "Take a heap dump now (jcmd/jmap), then Ctrl-C.");
        Thread.sleep(Long.MAX_VALUE); // keep the heap alive for dumping
    }
}
