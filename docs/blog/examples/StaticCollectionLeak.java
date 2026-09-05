/*
 * StaticCollectionLeak — the most common Java memory leak.
 *
 * THE BUG: a `static` collection is reachable from a GC root for the entire
 * life of the JVM. We keep adding to it and never remove anything, so every
 * object we put in is pinned forever. Eventually: OutOfMemoryError.
 *
 * THE FIX: bound the cache (size/time eviction, e.g. an LRU or Caffeine), or
 * don't keep it static, or clear entries when you're done with them.
 *
 * USED BY: blog #1 (what is an hprof), #3 (diagnosing OOM), #5 (common leaks).
 *
 * RUN:
 *   javac StaticCollectionLeak.java
 *   java -Xmx128m -XX:+HeapDumpOnOutOfMemoryError \
 *        -XX:HeapDumpPath=StaticCollectionLeak.hprof StaticCollectionLeak
 *
 * IN THE DUMP: the byte[] blocks dominate the heap, all retained through the
 * static List<byte[]> CACHE field — a textbook accumulation point with a short
 * GC-root chain.
 */
import java.util.ArrayList;
import java.util.List;

public class StaticCollectionLeak {

    // Rooted forever: a static field is a GC root's reachable target.
    static final List<byte[]> CACHE = new ArrayList<>();

    public static void main(String[] args) {
        System.out.println("Leaking into a static collection until OOM...");
        for (int i = 0; ; i++) {
            CACHE.add(new byte[1_000_000]); // ~1 MB, never released
            if (i % 50 == 0) {
                System.out.println("cached " + i + " blocks (~" + i + " MB)");
            }
        }
    }
}
