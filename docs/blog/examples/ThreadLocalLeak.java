/*
 * ThreadLocalLeak — a leak that hides on pooled threads.
 *
 * THE BUG: ThreadLocal values live as long as the *thread* does. On a thread
 * pool, threads are reused indefinitely, so a value you set and forget is never
 * released. Set big values on every task and you accumulate one per live worker
 * thread (and worse, across reuse if you overwrite without removing).
 *
 * THE FIX: always remove() in a finally block:
 *     try { TL.set(big); ... } finally { TL.remove(); }
 *
 * USED BY: blog #5 (common leaks).
 *
 * RUN:
 *   javac ThreadLocalLeak.java
 *   java -Xmx128m -XX:+HeapDumpOnOutOfMemoryError \
 *        -XX:HeapDumpPath=ThreadLocalLeak.hprof ThreadLocalLeak
 *
 * IN THE DUMP: the retained payloads hang off live worker Thread objects via
 * their threadLocals map — the GC-root chain runs through a thread, not your
 * own code, which is exactly why these are hard to spot by eye.
 */
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class ThreadLocalLeak {

    // Holds a big payload; never cleaned up after the task finishes.
    static final ThreadLocal<byte[]> CONTEXT = new ThreadLocal<>();

    public static void main(String[] args) throws InterruptedException {
        ExecutorService pool = Executors.newFixedThreadPool(8);
        System.out.println("Leaking ThreadLocal payloads on pooled threads...");
        for (int i = 0; i < 1_000_000; i++) {
            pool.submit(() -> {
                // BUG: set, but never remove(). The 2 MB payload stays attached
                // to whichever pool thread ran this task.
                CONTEXT.set(new byte[2_000_000]);
                // ... pretend to do work ...
            });
            if (i % 1000 == 0) Thread.sleep(1); // let the queue drain a bit
        }
        pool.shutdown();
    }
}
