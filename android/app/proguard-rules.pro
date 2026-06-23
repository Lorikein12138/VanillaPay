# Room
-keep class * extends androidx.room.RoomDatabase
-keep @androidx.room.Entity class *

# JSON / rule model fields used across config payloads
-keepclassmembers class com.vanillapay.monitor.parse.ParseRule { *; }
-keepclassmembers class com.vanillapay.monitor.parse.RuleSet { *; }

# ML Kit discovers barcode components through Firebase ComponentRegistrar
# implementations named in manifest metadata. AGP 9 / R8 full mode requires
# explicitly keeping reflective members, including public no-arg constructors.
-keep class * implements com.google.firebase.components.ComponentRegistrar { *; }
