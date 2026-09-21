import org.gradle.api.DefaultTask
import org.gradle.api.file.RegularFileProperty
import org.gradle.api.tasks.InputFile
import org.gradle.api.tasks.OutputFile
import org.gradle.api.tasks.TaskAction
import org.yaml.snakeyaml.Yaml

/**
 * Generates a jPOS GenericPackager XML descriptor from contracts/iso8583/packager-spec.yaml.
 *
 * Field-packager class names and the GenericPackager XML schema were confirmed against the
 * resolved jPOS 3.0.1 jar (MCN-102 schema probe), not assumed from older jPOS docs:
 * - DE 1 is an explicit <isofield> (jPOS's own bundled cfg/packager/iso87ascii.xml has one),
 *   using IFA_BITMAP with a fixed length of 16 hex chars; it auto-extends to the secondary
 *   bitmap when a DE >= 65 is present. The spec's own `fields[1]` entry (byte-oriented, for the
 *   Go codec's manual bitmap handling) does not apply here and is ignored for DE 1.
 * - jPOS 3.0.1 has no built-in LLL-prefixed binary class that keeps the payload as literal
 *   hex-ASCII text: IFA_LLLBINARY/IFB_LLLHEX use LiteralBinaryInterpreter, which packs raw bytes
 *   and corrupts an ASCII wire message. IFA_LLLHEXBINARY (this module,
 *   io.mcn.issuer.adapter.iso.IFA_LLLHEXBINARY) combines AsciiHexInterpreter with
 *   AsciiPrefixer.LLL to fill that gap, the same way jPOS's own IFA_BINARY combines
 *   AsciiHexInterpreter with NullPrefixer for fixed-length binary fields.
 * - Fixed-length an/ans fields use IF_CHAR, not IFA_ALPHA (that class does not exist in 3.0.1).
 */
abstract class GeneratePackagerTask : DefaultTask() {
    @get:InputFile
    abstract val specFile: RegularFileProperty

    @get:OutputFile
    abstract val outputFile: RegularFileProperty

    @TaskAction
    fun generate() {
        @Suppress("UNCHECKED_CAST")
        val spec = Yaml().load<Map<String, Any>>(specFile.get().asFile.reader())
        @Suppress("UNCHECKED_CAST")
        val mti = spec["mti"] as Map<String, Any>
        @Suppress("UNCHECKED_CAST")
        val fields = spec["fields"] as Map<Int, Map<String, Any>>

        val xml = StringBuilder()
        xml.append("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"no\"?>\n")
        xml.append("<!DOCTYPE isopackager PUBLIC\n")
        xml.append("        \"-//jPOS/jPOS Generic Packager DTD 1.0//EN\"\n")
        xml.append("        \"http://jpos.org/dtd/generic-packager-1.0.dtd\">\n\n")
        xml.append("<!-- Code generated from contracts/iso8583/packager-spec.yaml by :generatePackager. DO NOT EDIT. -->\n\n")
        xml.append("<isopackager>\n")
        xml.append(isofield(0, (mti["length"] as Int), "MESSAGE TYPE INDICATOR", "org.jpos.iso.IFA_NUMERIC"))
        xml.append(isofield(1, 16, "BIT MAP", "org.jpos.iso.IFA_BITMAP"))
        for ((number, f) in fields.toSortedMap()) {
            if (number == 1) continue // handled above: jPOS's own combined primary+secondary bitmap field
            val type = f["type"] as String
            val length = f["length"] as Int
            val prefix = f["prefix"] as String?
            val name = f["name"] as String
            xml.append(isofield(number, length, name.uppercase(), fieldPackagerClass(type, prefix)))
        }
        xml.append("</isopackager>\n")
        outputFile.get().asFile.writeText(xml.toString())
    }

    private fun isofield(id: Int, length: Int, name: String, packagerClass: String) =
        "  <isofield id=\"$id\" length=\"$length\" name=\"${escape(name)}\" class=\"$packagerClass\"/>\n"

    private fun fieldPackagerClass(type: String, prefix: String?): String = when {
        type == "n" && prefix == "LL" -> "org.jpos.iso.IFA_LLNUM"
        type == "n" && prefix == "LLL" -> "org.jpos.iso.IFA_LLLNUM"
        type == "n" && prefix == null -> "org.jpos.iso.IFA_NUMERIC"
        (type == "an" || type == "ans") && prefix == null -> "org.jpos.iso.IF_CHAR"
        (type == "an" || type == "ans") && prefix == "LLL" -> "org.jpos.iso.IFA_LLLCHAR"
        type == "b" && prefix == null -> "org.jpos.iso.IFA_BINARY"
        type == "b" && prefix == "LLL" -> "io.mcn.issuer.adapter.iso.IFA_LLLHEXBINARY"
        else -> error("unhandled type=$type prefix=$prefix")
    }

    private fun escape(s: String) = s.replace("&", "&amp;").replace("\"", "&quot;")
}
