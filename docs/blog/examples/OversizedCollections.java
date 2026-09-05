/*
 * OversizedCollections — memory waste from half-empty / over-sized collections.
 *
 * THE BUG: collections carry a backing array that's often much larger than the
 * number of elements actually stored. Pre-sizing a HashMap/ArrayList to a huge
 * capacity "just in case", or keeping millions of near-empty maps, wastes the
 * unused slots. Each HashMap also pays for a Node[] table plus per-entry Node
 * objects. At scale this dwarfs the data itself.
 *
 * THE FIX: size collections to their real load, use the right structure (a
 * primitive array, or a compact map), trimToSize() where appropriate, and avoid
 * "one small map per object" explosions.
 *
 * USED BY: blog #6 (why your HashMap/ArrayList is eating gigabytes).
 *
 * RUN (holds a steady state for dumping):
 *   javac OversizedCollections.java
 *   java -Xmx1g OversizedCollections &
 *   jcmd <pid> GC.heap_dump OversizedCollections.hprof
 *
 * IN THE DUMP: the Class Histogram shows a huge instance count of HashMap and
 * HashMap$Node[] backing tables (large shallow size), and HeapBuddy's
 * "Duplicates & Wasted" flags the inefficient, low-load-factor collections — all
 * on the default low-memory path, no dominator tree needed.
 */
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class OversizedCollections {

    public static void main(String[] args) throws InterruptedException {
        List<Map<String, String>> maps = new ArrayList<>();
        // One million maps, each pre-sized for 1000 entries but holding ~1.
        // The backing Node[] tables are almost entirely empty wasted slots.
        for (int i = 0; i < 1_000_000; i++) {
            Map<String, String> m = new HashMap<>(1000); // BUG: vastly over-sized
            m.put("k", "v");
            maps.add(m);
        }
        System.out.println("Holding " + maps.size() + " over-sized maps. "
                + "Take a heap dump now (jcmd/jmap), then Ctrl-C.");
        Thread.sleep(Long.MAX_VALUE);
    }
}
