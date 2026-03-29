package lang

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/j178/leetgo/config"
	"github.com/j178/leetgo/leetcode"
	"github.com/j178/leetgo/utils"
)

type java struct {
	baseLang
}

// extractReturnType 从代码片段中提取方法的返回类型
func (j java) extractReturnType(q *leetcode.QuestionData) string {
	codeSnippet := q.GetCodeSnippet(j.Slug())

	// 查找方法定义：public ReturnType methodName(
	// 例如：public List<List<String>> groupAnagrams(String[] strs)
	methodPattern := fmt.Sprintf("public\\s+(.+?)\\s+%s\\s*\\(", q.MetaData.Name)
	re := regexp.MustCompile(methodPattern)
	matches := re.FindStringSubmatch(codeSnippet)

	if len(matches) >= 2 {
		returnType := strings.TrimSpace(matches[1])
		// 移除可能的修饰符（如static, final等）
		parts := strings.Fields(returnType)
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// 如果提取失败，fallback到toJavaType
	return toJavaType(q.MetaData.Return.Type)
}

func toJavaType(typeName string) string {
	switch typeName {
	case "integer":
		return "int"
	case "string":
		return "String"
	case "long":
		return "long"
	case "double":
		return "double"
	case "boolean":
		return "boolean"
	case "character":
		return "char"
	case "void":
		return "void"
	case "TreeNode":
		return "TreeNode"
	case "ListNode":
		return "ListNode"
	default:
		if strings.HasSuffix(typeName, "[]") {
			return toJavaType(typeName[:len(typeName)-2]) + "[]"
		}
	}
	return typeName
}

func (j java) generateNormalTestCode(q *leetcode.QuestionData) string {
	// 生成参数声明和初始化
	paramDecls := make([]string, 0, len(q.MetaData.Params))
	paramNames := make([]string, 0, len(q.MetaData.Params))

	for _, param := range q.MetaData.Params {
		javaType := toJavaType(param.Type)
		paramDecls = append(
			paramDecls,
			fmt.Sprintf(
				"\t\t%s %s = LeetCodeIO.deserialize(%s.class, LeetCodeIO.readLine(br));",
				javaType,
				param.Name,
				javaType,
			),
		)
		paramNames = append(paramNames, param.Name)
	}

	// 生成方法调用
	var methodCall string
	var ansDecl string

	if q.MetaData.Return != nil && q.MetaData.Return.Type != "void" {
		// 有返回值 - 从代码片段中提取实际的返回类型
		returnType := j.extractReturnType(q)
		methodCall = fmt.Sprintf(
			"\t\t%s ans = new Solution().%s(%s);",
			returnType,
			q.MetaData.Name,
			strings.Join(paramNames, ", "),
		)
		ansDecl = ""
	} else {
		// 无返回值
		methodCall = fmt.Sprintf(
			"\t\tnew Solution().%s(%s);",
			q.MetaData.Name,
			strings.Join(paramNames, ", "),
		)
		if q.MetaData.Output != nil {
			// 使用修改后的参数作为输出
			ansDecl = fmt.Sprintf("\t\tObject ans = %s;", paramNames[q.MetaData.Output.ParamIndex])
		} else {
			ansDecl = "\t\tObject ans = null;"
		}
	}

	tpl := `class Main {
	public static void main(String[] args) throws Exception {
		BufferedReader br = new BufferedReader(new InputStreamReader(System.in));
%s
%s
%s
		System.out.println("\n%s " + LeetCodeIO.serialize(ans));
	}
}`

	return fmt.Sprintf(
		tpl,
		strings.Join(paramDecls, "\n"),
		methodCall,
		ansDecl,
		testCaseOutputMark,
	)
}

func (j java) generateSystemDesignTestCode(q *leetcode.QuestionData) string {
	className := q.MetaData.ClassName

	// 生成构造函数参数初始化
	constructorParams := ""
	constructorArgs := ""
	if len(q.MetaData.Constructor.Params) > 0 {
		ctorParamDecls := make([]string, 0)
		ctorParamNames := make([]string, 0)
		for i, param := range q.MetaData.Constructor.Params {
			javaType := toJavaType(param.Type)
			ctorParamDecls = append(
				ctorParamDecls,
				fmt.Sprintf(
					"\t\t\t%s %s = (%s) LeetCodeIO.convertToClass(%s.class, constructorParams.get(%d));",
					javaType,
					param.Name,
					javaType,
					javaType,
					i,
				),
			)
			ctorParamNames = append(ctorParamNames, param.Name)
		}
		constructorParams = "\t\t\tList<?> constructorParams = LeetCodeIO.asList(params.get(0));\n" +
			strings.Join(ctorParamDecls, "\n")
		constructorArgs = strings.Join(ctorParamNames, ", ")
	}

	// 生成每个方法的case
	methodCases := make([]string, 0)
	for _, method := range q.MetaData.Methods {
		caseCode := fmt.Sprintf("\t\t\t\tcase \"%s\": {", method.Name)

		// 方法参数初始化
		if len(method.Params) > 0 {
			caseCode += "\n\t\t\t\t\tList<?> methodParams = LeetCodeIO.asList(params.get(i));"
			for i, param := range method.Params {
				javaType := toJavaType(param.Type)
				caseCode += fmt.Sprintf(
					"\n\t\t\t\t\t%s %s = (%s) LeetCodeIO.convertToClass(%s.class, methodParams.get(%d));",
					javaType,
					param.Name,
					javaType,
					javaType,
					i,
				)
			}
		}

		// 方法调用
		methodParamNames := make([]string, 0)
		for _, param := range method.Params {
			methodParamNames = append(methodParamNames, param.Name)
		}

		if method.Return.Type != "" && method.Return.Type != "void" {
			caseCode += fmt.Sprintf(
				"\n\t\t\t\t\toutput[i] = obj.%s(%s);",
				method.Name,
				strings.Join(methodParamNames, ", "),
			)
		} else {
			caseCode += fmt.Sprintf(
				"\n\t\t\t\t\tobj.%s(%s);",
				method.Name,
				strings.Join(methodParamNames, ", "),
			)
			caseCode += "\n\t\t\t\t\toutput[i] = null;"
		}
		caseCode += "\n\t\t\t\t\tbreak;\n\t\t\t\t}"

		methodCases = append(methodCases, caseCode)
	}

	tpl := `class Main {
	public static void main(String[] args) throws Exception {
		BufferedReader br = new BufferedReader(new InputStreamReader(System.in));
		String[] ops = LeetCodeIO.deserialize(String[].class, LeetCodeIO.readLine(br));
		List<?> params = LeetCodeIO.asList(LeetCodeIO.parse(LeetCodeIO.readLine(br)));
		Object[] output = new Object[ops.length];

		// 构造函数
%s
		%s obj = new %s(%s);
		output[0] = null;

		// 方法调用
		for (int i = 1; i < ops.length; i++) {
			switch (ops[i]) {
%s
			}
		}

		System.out.println("\n%s " + LeetCodeIO.serialize(output));
	}
}`

	return fmt.Sprintf(
		tpl,
		constructorParams,
		className,
		className,
		constructorArgs,
		strings.Join(methodCases, "\n"),
		testCaseOutputMark,
	)
}

func hasClassDefinition(code, className string) bool {
	// Remove comments to avoid false positives from commented-out class definitions
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comment lines
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "/*") {
			continue
		}
		// Check for actual class/interface definition
		if strings.Contains(line, "class "+className+" ") ||
			strings.Contains(line, "class "+className+"{") ||
			strings.Contains(line, "interface "+className+" ") ||
			strings.Contains(line, "interface "+className+"{") {
			return true
		}
	}
	return false
}

func (j java) generateNodeDefs(q *leetcode.QuestionData) string {
	codeSnippet := q.GetCodeSnippet(j.Slug())
	nodeDefs := ""
	if !hasClassDefinition(codeSnippet, "ListNode") {
		nodeDefs += `
class ListNode {
	int val;
	ListNode next;
	ListNode() {}
	ListNode(int val) { this.val = val; }
	ListNode(int val, ListNode next) { this.val = val; this.next = next; }
}
`
	}
	if !hasClassDefinition(codeSnippet, "TreeNode") {
		nodeDefs += `
class TreeNode {
	int val;
	TreeNode left;
	TreeNode right;
	TreeNode() {}
	TreeNode(int val) { this.val = val; }
	TreeNode(int val, TreeNode left, TreeNode right) {
		this.val = val;
		this.left = left;
		this.right = right;
	}
}
`
	}
	return nodeDefs
}

func (j java) generateTestContent(q *leetcode.QuestionData) string {
	testMain := j.generateNormalTestCode(q)
	if q.MetaData.SystemDesign {
		testMain = j.generateSystemDesignTestCode(q)
	}

	helperCode := `
class LeetCodeIO {
	private LeetCodeIO() {}

	static final class Parser {
		private final String s;
		private int i;

		Parser(String s) {
			this.s = s;
			this.i = 0;
		}

		Object parseValue() {
			skipWhitespace();
			if (i >= s.length()) {
				return null;
			}
			char c = s.charAt(i);
			if (c == '[') {
				return parseArray();
			}
			if (c == '"') {
				return parseString();
			}
			if (c == 't') {
				expect("true");
				return Boolean.TRUE;
			}
			if (c == 'f') {
				expect("false");
				return Boolean.FALSE;
			}
			if (c == 'n') {
				expect("null");
				return null;
			}
			return parseNumber();
		}

		private List<Object> parseArray() {
			expectChar('[');
			skipWhitespace();
			List<Object> list = new ArrayList<>();
			if (peekChar(']')) {
				i++;
				return list;
			}
			while (i < s.length()) {
				list.add(parseValue());
				skipWhitespace();
				if (peekChar(',')) {
					i++;
					skipWhitespace();
					continue;
				}
				if (peekChar(']')) {
					i++;
					break;
				}
				throw new IllegalArgumentException("invalid array: " + s);
			}
			return list;
		}

		private String parseString() {
			expectChar('"');
			StringBuilder sb = new StringBuilder();
			while (i < s.length()) {
				char c = s.charAt(i++);
				if (c == '"') {
					break;
				}
				if (c == '\\') {
					if (i >= s.length()) {
						throw new IllegalArgumentException("invalid escape: " + s);
					}
					char n = s.charAt(i++);
					switch (n) {
						case '"': sb.append('"'); break;
						case '\\': sb.append('\\'); break;
						case '/': sb.append('/'); break;
						case 'b': sb.append('\b'); break;
						case 'f': sb.append('\f'); break;
						case 'n': sb.append('\n'); break;
						case 'r': sb.append('\r'); break;
						case 't': sb.append('\t'); break;
						case 'u':
							if (i + 4 > s.length()) {
								throw new IllegalArgumentException("invalid unicode escape: " + s);
							}
							String hex = s.substring(i, i + 4);
							i += 4;
							sb.append((char) Integer.parseInt(hex, 16));
							break;
						default:
							throw new IllegalArgumentException("invalid escape: \\" + n);
					}
				} else {
					sb.append(c);
				}
			}
			return sb.toString();
		}

		private Number parseNumber() {
			int start = i;
			if (s.charAt(i) == '-') {
				i++;
			}
			while (i < s.length() && Character.isDigit(s.charAt(i))) {
				i++;
			}
			boolean hasFraction = false;
			if (i < s.length() && s.charAt(i) == '.') {
				hasFraction = true;
				i++;
				while (i < s.length() && Character.isDigit(s.charAt(i))) {
					i++;
				}
			}
			if (i < s.length() && (s.charAt(i) == 'e' || s.charAt(i) == 'E')) {
				hasFraction = true;
				i++;
				if (i < s.length() && (s.charAt(i) == '+' || s.charAt(i) == '-')) {
					i++;
				}
				while (i < s.length() && Character.isDigit(s.charAt(i))) {
					i++;
				}
			}
			String num = s.substring(start, i);
			if (hasFraction) {
				return Double.parseDouble(num);
			}
			return Long.parseLong(num);
		}

		private void skipWhitespace() {
			while (i < s.length() && Character.isWhitespace(s.charAt(i))) {
				i++;
			}
		}

		private boolean peekChar(char c) {
			return i < s.length() && s.charAt(i) == c;
		}

		private void expect(String token) {
			if (!s.startsWith(token, i)) {
				throw new IllegalArgumentException("expected " + token + ", got: " + s.substring(i));
			}
			i += token.length();
		}

		private void expectChar(char c) {
			skipWhitespace();
			if (i >= s.length() || s.charAt(i) != c) {
				throw new IllegalArgumentException("expected '" + c + "', got: " + s);
			}
			i++;
		}
	}

	static String readLine(BufferedReader br) throws IOException {
		String line = br.readLine();
		if (line == null) {
			throw new EOFException("unexpected EOF");
		}
		return line.trim();
	}

	static Object parse(String raw) {
		return new Parser(raw).parseValue();
	}

	@SuppressWarnings("unchecked")
	static <T> T deserialize(Class<T> type, String raw) {
		Object parsed = parse(raw);
		return (T) convertToClass(type, parsed);
	}

	static List<?> asList(Object v) {
		if (v == null) {
			return Collections.emptyList();
		}
		if (!(v instanceof List<?>)) {
			throw new IllegalArgumentException("not an array: " + v);
		}
		return (List<?>) v;
	}

	static Constructor<?> findConstructor(Class<?> cls, int argc) {
		for (Constructor<?> ctor : cls.getDeclaredConstructors()) {
			if (ctor.getParameterCount() == argc) {
				return ctor;
			}
		}
		throw new IllegalArgumentException("constructor not found for " + cls.getName() + ", argc=" + argc);
	}

	static Method findMethod(Class<?> cls, String name, int argc) {
		for (Method m : cls.getDeclaredMethods()) {
			if (m.getName().equals(name) && m.getParameterCount() == argc) {
				return m;
			}
		}
		throw new IllegalArgumentException("method not found: " + name + ", argc=" + argc);
	}

	static Object[] convertArguments(Class<?>[] paramTypes, List<?> params) {
		Object[] args = new Object[paramTypes.length];
		for (int i = 0; i < paramTypes.length; i++) {
			Object raw = i < params.size() ? params.get(i) : null;
			args[i] = convertToClass(paramTypes[i], raw);
		}
		return args;
	}

	static Object convertToType(String typeName, Object raw) {
		if (typeName.endsWith("[]")) {
			String elemType = typeName.substring(0, typeName.length() - 2);
			List<?> list = asList(raw);
			Class<?> elemClass = classOf(elemType);
			Object arr = Array.newInstance(elemClass, list.size());
			for (int i = 0; i < list.size(); i++) {
				Array.set(arr, i, convertToType(elemType, list.get(i)));
			}
			return arr;
		}
		switch (typeName) {
			case "integer":
				return ((Number) raw).intValue();
			case "long":
				return ((Number) raw).longValue();
			case "double":
				return ((Number) raw).doubleValue();
			case "boolean":
				return (Boolean) raw;
			case "string":
				return raw == null ? null : raw.toString();
			case "character": {
				if (raw instanceof String) {
					String s = (String) raw;
					return s.isEmpty() ? '\u0000' : s.charAt(0);
				}
				return (char) ((Number) raw).intValue();
			}
			case "TreeNode":
				return buildTree(asList(raw));
			case "ListNode":
				return buildList(asList(raw));
			default:
				return raw;
		}
	}

	static Object convertToClass(Class<?> cls, Object raw) {
		if (cls.isArray()) {
			List<?> list = asList(raw);
			Class<?> comp = cls.getComponentType();
			Object arr = Array.newInstance(comp, list.size());
			for (int i = 0; i < list.size(); i++) {
				Array.set(arr, i, convertToClass(comp, list.get(i)));
			}
			return arr;
		}
		if (cls == int.class || cls == Integer.class) {
			return ((Number) raw).intValue();
		}
		if (cls == long.class || cls == Long.class) {
			return ((Number) raw).longValue();
		}
		if (cls == double.class || cls == Double.class) {
			return ((Number) raw).doubleValue();
		}
		if (cls == boolean.class || cls == Boolean.class) {
			return (Boolean) raw;
		}
		if (cls == char.class || cls == Character.class) {
			if (raw instanceof String) {
				String s = (String) raw;
				return s.isEmpty() ? '\u0000' : s.charAt(0);
			}
			return (char) ((Number) raw).intValue();
		}
		if (cls == String.class) {
			return raw == null ? null : raw.toString();
		}
		if (cls == TreeNode.class) {
			return buildTree(asList(raw));
		}
		if (cls == ListNode.class) {
			return buildList(asList(raw));
		}
		return raw;
	}

	static Class<?> classOf(String typeName) {
		if (typeName.endsWith("[]")) {
			Class<?> elem = classOf(typeName.substring(0, typeName.length() - 2));
			return Array.newInstance(elem, 0).getClass();
		}
		switch (typeName) {
			case "integer":
				return int.class;
			case "long":
				return long.class;
			case "double":
				return double.class;
			case "boolean":
				return boolean.class;
			case "character":
				return char.class;
			case "string":
				return String.class;
			case "TreeNode":
				return TreeNode.class;
			case "ListNode":
				return ListNode.class;
			default:
				return Object.class;
		}
	}

	static ListNode buildList(List<?> arr) {
		ListNode dummy = new ListNode(0);
		ListNode cur = dummy;
		for (Object v : arr) {
			cur.next = new ListNode(v == null ? 0 : ((Number) v).intValue());
			cur = cur.next;
		}
		return dummy.next;
	}

	static TreeNode buildTree(List<?> arr) {
		if (arr.isEmpty() || arr.get(0) == null) {
			return null;
		}
		TreeNode root = new TreeNode(((Number) arr.get(0)).intValue());
		Queue<TreeNode> q = new ArrayDeque<>();
		q.offer(root);
		int idx = 1;
		while (!q.isEmpty() && idx < arr.size()) {
			TreeNode cur = q.poll();
			Object left = arr.get(idx++);
			if (left != null) {
				cur.left = new TreeNode(((Number) left).intValue());
				q.offer(cur.left);
			}
			if (idx >= arr.size()) {
				break;
			}
			Object right = arr.get(idx++);
			if (right != null) {
				cur.right = new TreeNode(((Number) right).intValue());
				q.offer(cur.right);
			}
		}
		return root;
	}

	static String serialize(Object v) {
		StringBuilder sb = new StringBuilder();
		appendValue(sb, v);
		return sb.toString();
	}

	static void appendValue(StringBuilder sb, Object v) {
		if (v == null) {
			sb.append("null");
			return;
		}
		if (v instanceof String) {
			appendQuoted(sb, (String) v);
			return;
		}
		if (v instanceof Character) {
			appendQuoted(sb, String.valueOf(v));
			return;
		}
		if (v instanceof Number || v instanceof Boolean) {
			sb.append(v);
			return;
		}
		if (v instanceof TreeNode) {
			appendTree(sb, (TreeNode) v);
			return;
		}
		if (v instanceof ListNode) {
			appendListNode(sb, (ListNode) v);
			return;
		}
		Class<?> cls = v.getClass();
		if (cls.isArray()) {
			sb.append('[');
			int n = Array.getLength(v);
			for (int i = 0; i < n; i++) {
				if (i > 0) {
					sb.append(',');
				}
				appendValue(sb, Array.get(v, i));
			}
			sb.append(']');
			return;
		}
		if (v instanceof List<?>) {
			List<?> list = (List<?>) v;
			sb.append('[');
			for (int i = 0; i < list.size(); i++) {
				if (i > 0) {
					sb.append(',');
				}
				appendValue(sb, list.get(i));
			}
			sb.append(']');
			return;
		}
		appendQuoted(sb, v.toString());
	}

	static void appendQuoted(StringBuilder sb, String s) {
		sb.append('"');
		for (int i = 0; i < s.length(); i++) {
			char c = s.charAt(i);
			switch (c) {
				case '"': sb.append("\\\\\""); break;
				case '\\': sb.append("\\\\\\\\"); break;
				case '\b': sb.append("\\\\b"); break;
				case '\f': sb.append("\\\\f"); break;
				case '\n': sb.append("\\\\n"); break;
				case '\r': sb.append("\\\\r"); break;
				case '\t': sb.append("\\\\t"); break;
				default:
					if (c < 0x20) {
						sb.append(String.format("\\\\u%04x", (int) c));
					} else {
						sb.append(c);
					}
			}
		}
		sb.append('"');
	}

	static void appendListNode(StringBuilder sb, ListNode head) {
		sb.append('[');
		boolean first = true;
		for (ListNode cur = head; cur != null; cur = cur.next) {
			if (!first) {
				sb.append(',');
			}
			first = false;
			sb.append(cur.val);
		}
		sb.append(']');
	}

	static void appendTree(StringBuilder sb, TreeNode root) {
		if (root == null) {
			sb.append("[]");
			return;
		}
		List<String> vals = new ArrayList<>();
		LinkedList<TreeNode> q = new LinkedList<>();
		q.offer(root);
		while (!q.isEmpty()) {
			TreeNode cur = q.poll();
			if (cur == null) {
				vals.add("null");
				continue;
			}
			vals.add(String.valueOf(cur.val));
			q.offer(cur.left);
			q.offer(cur.right);
		}
		int end = vals.size() - 1;
		while (end >= 0 && "null".equals(vals.get(end))) {
			end--;
		}
		sb.append('[');
		for (int i = 0; i <= end; i++) {
			if (i > 0) {
				sb.append(',');
			}
			sb.append(vals.get(i));
		}
		sb.append(']');
	}
}
`

	parts := []string{helperCode, testMain}
	content := strings.Join(parts, "\n")
	if q.MetaData.Manual {
		content = fmt.Sprintf("// %s\n%s", manualWarning, content)
	}
	return content
}

func (j java) generateCodeFile(
	q *leetcode.QuestionData,
	filename string,
	blocks []config.Block,
	modifiers []ModifierFunc,
	separateDescriptionFile bool,
) (
	FileOutput,
	error,
) {
	codeHeader := `import java.io.*;
import java.lang.reflect.*;
import java.util.*;
`
	nodeDefs := j.generateNodeDefs(q)
	testContent := j.generateTestContent(q)
	// 将nodeDefs和testContent合并到afterAfterMarker
	// Java允许类在使用它的类之后定义，所以顺序不重要
	afterContent := testContent + "\n" + nodeDefs
	blocks = append(
		[]config.Block{
			{
				Name:     beforeBeforeMarker,
				Template: codeHeader,
			},
			{
				Name:     afterAfterMarker,
				Template: afterContent,
			},
		},
		blocks...,
	)
	content, err := j.generateCodeContent(
		q,
		blocks,
		modifiers,
		separateDescriptionFile,
	)
	if err != nil {
		return FileOutput{}, err
	}
	return FileOutput{
		Filename: filename,
		Content:  content,
		Type:     CodeFile | TestFile,
	}, nil
}

func (j java) RunLocalTest(q *leetcode.QuestionData, outDir string, targetCase string) (bool, error) {
	genResult, err := j.GeneratePaths(q)
	if err != nil {
		return false, fmt.Errorf("generate paths failed: %w", err)
	}
	genResult.SetOutDir(outDir)

	workDir := genResult.TargetDir()
	localResult := &GenerateResult{
		Question: q,
		Lang:     j,
		OutDir:   workDir,
	}
	localResult.AddFile(
		FileOutput{
			Filename: filepath.Base(genResult.GetFile(TestFile).GetPath()),
			Type:     CodeFile | TestFile,
		},
	)

	testFile := localResult.GetFile(TestFile).GetPath()
	if !utils.IsExist(testFile) {
		return false, fmt.Errorf("test file %s not found", utils.RelToCwd(testFile))
	}

	defaultTestCasesName := filepath.Base(genResult.GetFile(TestCasesFile).GetPath())
	testCasesName := defaultTestCasesName
	testCasesPath := filepath.Join(workDir, testCasesName)
	if !utils.IsExist(testCasesPath) {
		legacyPath := filepath.Join(workDir, "testcases.txt")
		if utils.IsExist(legacyPath) {
			testCasesName = "testcases.txt"
		} else {
			// Auto-generate testcases file for compatibility with previously generated Java files.
			tc, tcErr := j.generateTestCasesFile(q, defaultTestCasesName)
			if tcErr != nil {
				return false, fmt.Errorf("generate testcases failed: %w", tcErr)
			}
			if writeErr := utils.WriteFile(testCasesPath, []byte(tc.Content)); writeErr != nil {
				return false, fmt.Errorf("write testcases file %s failed: %w", utils.RelToCwd(testCasesPath), writeErr)
			}
		}
	}
	localResult.AddFile(FileOutput{Filename: testCasesName, Type: TestCasesFile})

	// 创建build目录并添加.gitignore
	buildDir := filepath.Join(workDir, "build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return false, fmt.Errorf("create build directory failed: %w", err)
	}

	// 创建.gitignore忽略build目录
	gitignorePath := filepath.Join(workDir, ".gitignore")
	if !utils.IsExist(gitignorePath) {
		gitignoreContent := "build/\n*.class\n"
		if err := utils.WriteFile(gitignorePath, []byte(gitignoreContent)); err != nil {
			// 非关键错误，只记录日志
			log.Warn("failed to create .gitignore", "error", err)
		}
	}

	// 编译到build目录
	err = buildTest(q, localResult, []string{"javac", "-d", "build", filepath.Base(testFile)})
	if err != nil {
		return false, fmt.Errorf("build failed: %w", err)
	}

	// 从build目录运行
	return runTest(q, localResult, []string{"java", "-cp", "build", "Main"}, targetCase)
}

func (j java) GeneratePaths(q *leetcode.QuestionData) (*GenerateResult, error) {
	filenameTmpl := getFilenameTemplate(q, j)
	baseFilename, err := q.GetFormattedFilename(j.slug, filenameTmpl)
	if err != nil {
		return nil, err
	}
	genResult := &GenerateResult{
		SubDir:   baseFilename,
		Question: q,
		Lang:     j,
	}
	genResult.AddFile(
		FileOutput{
			Filename: "solution.java",
			Type:     CodeFile | TestFile,
		},
	)
	genResult.AddFile(
		FileOutput{
			Filename: "testcases.txt",
			Type:     TestCasesFile,
		},
	)
	if separateDescriptionFile(j) {
		genResult.AddFile(
			FileOutput{
				Filename: "question.md",
				Type:     DocFile,
			},
		)
	}
	return genResult, nil
}

func (j java) Generate(q *leetcode.QuestionData) (*GenerateResult, error) {
	filenameTmpl := getFilenameTemplate(q, j)
	baseFilename, err := q.GetFormattedFilename(j.slug, filenameTmpl)
	if err != nil {
		return nil, err
	}
	genResult := &GenerateResult{
		Question: q,
		Lang:     j,
		SubDir:   baseFilename,
	}

	separateDescriptionFile := separateDescriptionFile(j)
	blocks := getBlocks(j)
	modifiers, err := getModifiers(j, builtinModifiers)
	if err != nil {
		return nil, err
	}
	codeFile, err := j.generateCodeFile(q, "solution.java", blocks, modifiers, separateDescriptionFile)
	if err != nil {
		return nil, err
	}
	testcaseFile, err := j.generateTestCasesFile(q, "testcases.txt")
	if err != nil {
		return nil, err
	}
	genResult.AddFile(codeFile)
	genResult.AddFile(testcaseFile)

	if separateDescriptionFile {
		docFile, err := j.generateDescriptionFile(q, "question.md")
		if err != nil {
			return nil, err
		}
		genResult.AddFile(docFile)
	}

	return genResult, nil
}
