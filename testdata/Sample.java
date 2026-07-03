package com.example.app;

import java.util.List;
import java.util.Map;
import java.io.*;
import static java.util.Objects.requireNonNull;

public class UserService extends BaseService implements AutoCloseable {
    public static final String VERSION = "1.0";
    private final Map<String, User> cache = Map.of();
    protected int retryCount;
    int packagePrivateField;

    public UserService(List<User> initial) {
        super();
    }

    public User findById(String id) throws IOException {
        return cache.get(id);
    }

    @Override
    public void close() {
    }

    private void evict(String id) {
    }

    public static <T extends Comparable<T>> T max(List<T> items) {
        return items.get(0);
    }
}

interface Repository<T> {
    T load(String id);

    default boolean exists(String id) {
        return load(id) != null;
    }
}

enum Status {
    ACTIVE,
    SUSPENDED,
    DELETED;

    public boolean isTerminal() {
        return this == DELETED;
    }
}

record Point(int x, int y) {
    public double distance() {
        return Math.sqrt(x * x + y * y);
    }
}

@interface Tag {
    String value();
}

class PackagePrivateHelper {
    void help() {
    }
}
