package io.mcn.issuer.adapter.logging;

import static java.nio.charset.StandardCharsets.UTF_8;
import static org.assertj.core.api.Assertions.assertThat;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.LoggerContext;
import ch.qos.logback.classic.joran.JoranConfigurator;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.classic.spi.LoggingEvent;
import ch.qos.logback.core.OutputStreamAppender;
import org.junit.jupiter.api.Test;

class LogbackMaskingTest {

  @Test
  void should_write_masked_json_with_standard_fields__MCN_005_AC2() throws Exception {
    LoggerContext context = new LoggerContext();
    JoranConfigurator configurator = new JoranConfigurator();
    configurator.setContext(context);
    configurator.doConfigure(getClass().getResource("/logback.xml"));

    @SuppressWarnings("unchecked")
    var appender =
        (OutputStreamAppender<ILoggingEvent>) context.getLogger("ROOT").getAppender("JSON");
    var event =
        new LoggingEvent(
            "test", context.getLogger("t"), Level.INFO, "authorized 9704360000004417", null, null);
    // A hand-built LoggingEvent has no MDC adapter attached (only a real Logger.info() call would
    // set one); logstash-logback-encoder 9's MdcJsonProvider NPEs reading it otherwise.
    event.setMDCPropertyMap(java.util.Collections.emptyMap());

    String json = new String(appender.getEncoder().encode(event), UTF_8);

    assertThat(json)
        .contains("\"ts\"")
        .contains("\"service\":\"issuer\"")
        .contains("authorized 970436******4417")
        .doesNotContain("9704360000004417");
  }
}
