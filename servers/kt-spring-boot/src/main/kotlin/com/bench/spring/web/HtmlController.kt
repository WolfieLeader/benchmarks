package com.bench.spring.web

import org.springframework.stereotype.Controller
import org.springframework.ui.Model
import org.springframework.web.bind.annotation.GetMapping

/**
 * `GET /html` → a server-rendered Thymeleaf template (`templates/index.html`). A
 * plain `@Controller` returning the view name renders HTML (`text/html`); the contract
 * asserts the interpolated greeting, fruit list, and labeled total via `htmlContains`.
 */
@Controller
class HtmlController {
    @GetMapping("/html")
    fun html(model: Model): String {
        model.addAttribute("name", "Alice")
        model.addAttribute("fruits", listOf("apple", "banana", "cherry"))
        model.addAttribute("total", HTML_TOTAL)
        return "index"
    }

    private companion object {
        const val HTML_TOTAL = 42
    }
}
