using System;
using System.IO;
using System.Diagnostics;

public static class OwnedVersionOnly
{
    public static int Main(string[] args)
    {
        bool versionOnly = args.Length == 1 && args[0] == "--version";
        string observation = "{\"pid\":" + Process.GetCurrentProcess().Id +
            ",\"version_only\":" + (versionOnly ? "true" : "false") + "}\n";
        File.AppendAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "version-probes.jsonl"), observation);
        if (!versionOnly) { Console.Error.WriteLine("Owned fixture permits version inspection only"); return 64; }
        Console.WriteLine("2.1.314 (Claude Code)");
        return 0;
    }
}
