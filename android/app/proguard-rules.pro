# Room
-keep class * extends androidx.room.RoomDatabase
-keep @androidx.room.Entity class *

# JSON / rule model fields used across config payloads
-keepclassmembers class com.vanillapay.monitor.parse.ParseRule { *; }
-keepclassmembers class com.vanillapay.monitor.parse.RuleSet { *; }
