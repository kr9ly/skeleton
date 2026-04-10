package com.example.sample

import kotlin.collections.List
import java.util.Map

typealias StringList = List<String>

val topLevelVal: Int = 42
var topLevelVar: String = "hello"

private val hiddenVal: Boolean = false
internal var hiddenVar: Long = 0L

fun greet(name: String): String {
    return "Hello, $name"
}

fun add(a: Int, b: Int): Int = a + b

private fun secretFunc(): Unit {}
internal fun internalHelper(): Unit {}

data class Person(
    val name: String,
    val age: Int
)

class Repository(val baseUrl: String) {
    val timeout: Int = 30
    private val secret: String = ""

    fun fetch(path: String): String {
        return baseUrl + path
    }

    fun post(path: String, body: String): Boolean = true

    private fun authenticate(): Unit {}
}

sealed class Result {
    data class Success(val data: String) : Result()
    data class Failure(val error: String) : Result()
    object Loading : Result()
}

enum class Status {
    PENDING, ACTIVE, INACTIVE;

    fun isActive(): Boolean = this == ACTIVE
}

interface Serializable {
    val id: String
    fun serialize(): String
    fun deserialize(data: String): Unit {
        println(data)
    }
}

object AppConfig {
    val version: String = "1.0.0"
    val debug: Boolean = false

    fun getVersion(): String = version
}
