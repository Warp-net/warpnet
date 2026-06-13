.PHONY: build
build:
	./gradlew clean && ./gradlew build -x test -x lint -x lintAnalyzeDebug -x lintReportDebug
	cd node && ./build-native.sh && cd -



