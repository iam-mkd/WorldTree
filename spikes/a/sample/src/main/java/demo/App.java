package demo;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.*;
import java.util.*;

@SpringBootApplication
@RestController
public class App {
    private final JdbcTemplate db; private final StringRedisTemplate redis;
    App(JdbcTemplate db, StringRedisTemplate redis) { this.db = db; this.redis = redis; }
    public static void main(String[] a) { SpringApplication.run(App.class, a); }

    @PostMapping("/notes") Map<String,Object> add(@RequestParam String text) {
        db.update("insert into note(text) values (?)", text);
        return Map.of("hits", redis.opsForValue().increment("notes:writes"));
    }
    @GetMapping("/notes") Map<String,Object> list() {
        return Map.of("notes", db.queryForList("select text from note order by id", String.class),
                      "writes", String.valueOf(redis.opsForValue().get("notes:writes")));
    }
}
