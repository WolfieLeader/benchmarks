package com.bench.spring.web

import org.springframework.http.MediaType
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RestController

/** `GET /` → `{"hello":"world"}` (JSON); `GET /health` → `OK` (text/plain). */
@RestController
class RootController {
    @GetMapping("/")
    fun root(): HelloResponse = HelloResponse("world")

    @GetMapping("/health")
    fun health(): ResponseEntity<String> = ResponseEntity.ok().contentType(MediaType.TEXT_PLAIN).body("OK")
}
