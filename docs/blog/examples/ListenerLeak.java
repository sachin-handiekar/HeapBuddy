/*
 * ListenerLeak — the "forgot to unregister" leak.
 *
 * THE BUG: a long-lived publisher holds strong references to every listener
 * that subscribes. If subscribers come and go (requests, screens, sessions) but
 * never unsubscribe, the publisher pins them — and everything they retain —
 * forever. This is the leak behind a surprising share of real-world OOMs, and
 * the Android "leaked Activity" is a special case of it.
 *
 * THE FIX: unregister in the symmetric teardown (close/dispose/onDestroy), or
 * hold listeners weakly, or use a lifecycle-aware bus.
 *
 * USED BY: blog #5 (common leaks); referenced by #8 (Android).
 *
 * RUN:
 *   javac ListenerLeak.java
 *   java -Xmx128m -XX:+HeapDumpOnOutOfMemoryError \
 *        -XX:HeapDumpPath=ListenerLeak.hprof ListenerLeak
 *
 * IN THE DUMP: a single EventBus retains a growing List of Subscriber objects,
 * each dragging a 512 KB payload. The accumulation point is the bus's listener
 * list; the GC-root chain ends at the static BUS.
 */
import java.util.ArrayList;
import java.util.List;

public class ListenerLeak {

    interface Listener { void onEvent(String e); }

    static final class EventBus {
        private final List<Listener> listeners = new ArrayList<>();
        void register(Listener l)   { listeners.add(l); }      // strong ref
        void unregister(Listener l) { listeners.remove(l); }   // ...never called
        void publish(String e)      { for (Listener l : listeners) l.onEvent(e); }
    }

    // A subscriber that retains some memory of its own.
    static final class Subscriber implements Listener {
        private final byte[] payload = new byte[512_000]; // 512 KB
        public void onEvent(String e) { /* ... */ }
    }

    static final EventBus BUS = new EventBus();

    public static void main(String[] args) {
        System.out.println("Registering listeners and never unregistering...");
        for (int i = 0; ; i++) {
            BUS.register(new Subscriber()); // BUG: no matching unregister()
            if (i % 100 == 0) System.out.println("registered " + i + " listeners");
        }
    }
}
