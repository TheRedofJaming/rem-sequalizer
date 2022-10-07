package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var ignoreList = []string{
	"Document Sidebar",
	"List Item",
	"Daily Document",
	"Quick Add",
	"Card Item",
	"Custom CSS",
	"Source List",
	"Suspend Cards",
	"Link",
	"Template Slot",
	"~",
}
var (
	OUTDIR        = ""
	INDIR         = "./rem.json"
	PRINTLOG      = false
	PAGEREFERENCE = false
)

// Each Rem is a whole js object. "Content" is a copy of it, missing only the redundant "_id" filed. The other fields are processing related extractions.
type blockNode struct {
	Id              string
	ParentBlock     *blockNode
	SubBlocks       []*blockNode
	Content         map[string]any
	StringedContent string
	HeadKey         string
	Doc             bool
}

func (b *blockNode) String() string {
	return b.HeadKey
}

func main() {
	Flags()

	if !PRINTLOG {
		log.SetOutput(io.Discard)
	}
	data, err := ImportRemData(INDIR)
	log.Print(INDIR)
	if err != nil {
		log.Fatal("Importing data failed:", err)
	}

	log.Println("JSON Loaded")

	blocks, err := CreateAllBlocks(data)
	if err != nil {
		log.Fatal(err)
	}

	topLevelBlocks, err := CreateBlockTree(blocks)
	if err != nil {
		log.Fatal(err)
	}
	// parse block content
	for id := range blocks {
		if err := blocks[id].parseContent(&blocks); err != nil {
			log.Fatal(err)
		}
	}
	// create output folder
	if _, err := os.Stat(OUTDIR); os.IsNotExist(err) {
		err := os.Mkdir(OUTDIR, os.ModePerm)
		if err != nil {
			log.Fatalf("\"%s\" Folder creation went wrong. %v", OUTDIR, err)
		}
	}
	// traverse blocks and create file structure
	for _, node := range topLevelBlocks {
		if node.HeadKey == "" {
			continue
		}
		file, err := os.Create(OUTDIR + "/" + correctFileName(node.HeadKey) + ".md")
		if err != nil {
			log.Fatalf("\"%s\" Entry-File creation went wrong", err)
		}

		w := bufio.NewWriter(file)
		defer file.Close()
		if err := walkNodes(node, w); err != nil {
			log.Fatal("Node walk failed:", err)
		}
		w.Flush()
	}
}

// parse cmdline flags
func Flags() {
	flag.StringVar(&OUTDIR, "o", "pages", "Output directory. Creates \"pages\" dir by default.")
	flag.StringVar(&INDIR, "i", "rem.json", "Input file (usually rem.json). Looks for \"rem.json\" in the executables directory by default.")
	flag.BoolVar(&PRINTLOG, "v", false, "Verbose output.")
	flag.BoolVar(&PAGEREFERENCE, "p", false, "Employ Page References for structuring. Recommended for vast datasets, as (cascading) Page Embeds are slow in Logseq.")
	flag.Parse()
}

// Function importing data and converting json to go map
func ImportRemData(filepath string) (map[string]any, error) {
	content, err := ioutil.ReadFile(filepath)
	if err != nil {
		return map[string]any{}, err
	}
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return map[string]any{}, err
	}
	return data, nil
}

func inFilter(s string) bool {
	for _, c := range ignoreList {
		if s == c {
			return true
		}
	}
	return false
}

// Function which converts the in-list rem to id-mapped blocknodes
func CreateAllBlocks(data map[string]any) (map[string]*blockNode, error) {
	// get rem
	rawRems, ok := data["docs"].([]any)
	if !ok {
		return map[string]*blockNode{}, fmt.Errorf("Accessing docs went wrong.")
	}
	blocks := make(map[string]*blockNode)
	// create blocks from map
	for _, rawRem := range rawRems {
		rem, ok := rawRem.(map[string]any)
		if !ok {
			return map[string]*blockNode{}, fmt.Errorf("Bulletpoint type assertion failed.")
		}
		keys, ok := rem["key"].([]any)
		if ok && len(keys) > 0 {
			firstKey, ok := keys[0].(string)
			if ok {
				if inFilter(firstKey) {
					continue
				}
			}
		}

		id, ok := rem["_id"].(string)
		if !ok {
			return map[string]*blockNode{}, fmt.Errorf("Bulletpoint id extraction failed.")
		}
		isDoc := false
		_, exists := rem["docUpdated"]
		if exists {
			//("Found document: %s", id)
			isDoc = true
		}
		delete(rem, "_id")
		blocks[id] = &blockNode{
			Id:          id,
			ParentBlock: nil,
			SubBlocks:   []*blockNode{},
			Content:     rem,
			HeadKey:     "",
			Doc:         isDoc,
		}
	}
	return blocks, nil
}
func CreateBlockTree(blocks map[string]*blockNode) ([]*blockNode, error) {
	// create block tree
	for _, node := range blocks {
		if err := node.resolveHeadKey(); err != nil {
			return []*blockNode{}, err
		}
	}
	topLevelBlocks := []*blockNode{}
	for _, node := range blocks {
		if err := node.resolveSubBlocks(&blocks); err != nil {
			return []*blockNode{}, err
		}
		err, isTopNode := node.resolveParent(blocks)
		if err != nil {
			return []*blockNode{}, err
		}
		if isTopNode {
			topLevelBlocks = append(topLevelBlocks, node)
		}
	}

	return topLevelBlocks, nil
}

// Function which translates the id-string list to actual node pointers.
func (b *blockNode) resolveSubBlocks(index *map[string]*blockNode) error {
	subBlocksForId, ok := b.Content["subBlocks"].([]any)
	if ok != true {
		return fmt.Errorf("Could not assert subBlocks for %s: %v\n", b.Id, subBlocksForId)
	}

	for i := len(subBlocksForId) - 1; i > -1; i-- {
		subBlockString, ok := subBlocksForId[i].(string)
		if !ok {
			return fmt.Errorf("Could not assert subBlock string for %s: %v\n", b.Id, subBlocksForId)
		}
		subBlockNode := (*index)[subBlockString]
		b.SubBlocks = append(b.SubBlocks, subBlockNode)
	}
	return nil
}

// Function to determine the parent of a nodeblock.
// Usefull to identify top-level nodes
func (b *blockNode) resolveParent(index map[string]*blockNode) (error, bool) {
	switch parentForId := b.Content["parent"].(type) {
	case nil:
		// fmt.Printf("%v is a toplevel node\n", b.Id)
		b.ParentBlock = nil
		return nil, true
	case string:
		b.ParentBlock = index[parentForId]
	default:
		return fmt.Errorf("Could not assert parent for %s\n", b.Id), false
	}
	return nil, false
}

// Wrapperfunction for walking the tree
func walkNodes(node *blockNode, w *bufio.Writer) error {
	visited := make(map[*blockNode]bool)
	log.Println("Started Walking.")
	err := walk(&visited, node, 0, w)
	log.Println("Finished Walking.")
	return err
}

// if Doc == true -> page embed, new file -> continue parsing
// pass in new writer in walk function

// Function which walks the tree recursively
func walk(visited *map[*blockNode]bool, node *blockNode, level int, w *bufio.Writer) error {
	// to avoid nil pointers
	if node == nil {
		return nil
	}
	// check if node has been visited
	if (*visited)[node] == true {
		return nil
	} else {
		(*visited)[node] = true
		//("Node %s has been visited.", node.HeadKey)
	}
	if node.Doc && node.ParentBlock != nil {
		filename := correctFileName(node.HeadKey)
		filepath := OUTDIR + "/" + filename + ".md"
		// write pageembed in current file
		if err := WritePeOrRef(node, level, w); err != nil {
			return err
		}
		file, err := os.Create(filepath)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("\"%s\" File creation went wrong", err.Error()))
		}
		defer file.Close()
		// From now on write everything to new file with level 0 again
		level = 0
		w = bufio.NewWriter(file)
	}
	if err := WriteContent(node, level, w); err != nil {
		return err
	}
	for i := 0; i < len(node.SubBlocks); i++ {
		// if the node is a Document, create a new file and write page embed
		next := node.SubBlocks[i]
		if next == nil {
			log.Printf("Sublocks for %s are empty.", node.HeadKey)
			continue
		}
		if err := walk(visited, next, level+1, w); err != nil {
			return err
		}
		// }
	}
	w.Flush()
	return nil
}

// Function which always emits a valid filepath, regardless of special characters
func correctFileName(name string) string {
	specialChars := []string{" ", ":", "#", "-", "%", "$", "§", "@", "!", "()", "{}", "[]", "<", ">", "."}
	hasSpecialChar := false
	for _, c := range specialChars {
		hasSpecialChar = strings.Contains(name, c)
	}
	if hasSpecialChar {
		name = strings.Join([]string{"'", name, "'"}, "")
	}
	if strings.Contains(name, "/") {
		name = strings.ReplaceAll(name, "/", "|")
	}
	return name
}

// Writes Page Embed
func WritePeOrRef(node *blockNode, level int, w io.Writer) error {
	buildString := new(strings.Builder)
	for i := 0; i < level; i++ {
		buildString.WriteRune('\t')
	}
	buildString.WriteString("- ")
	if PAGEREFERENCE {
		buildString.WriteString(strings.Join([]string{"[[", node.HeadKey, "]]\n"}, ""))
	} else {
		buildString.WriteString(strings.Join([]string{"{{embed [[", node.HeadKey, "]]}}\n"}, ""))
	}
	_, err := w.Write([]byte(buildString.String()))
	if err != nil {
		return err
	}
	return nil
}

// Function, handling indentation to write content
func WriteContent(node *blockNode, level int, w io.Writer) error {
	if content := []rune(node.StringedContent); content != nil {
		buildString := new(strings.Builder)
		for i := 0; i < level; i++ {
			buildString.WriteRune('\t')
		}
		buildString.WriteString("- ")
		for _, r := range content {
			buildString.WriteRune(r)
			if r == '\n' {
				for i := 0; i < level; i++ {
					buildString.WriteRune('\t')
				}
			}
		}
		buildString.WriteRune('\n')
		_, err := w.Write([]byte(buildString.String()))
		if err != nil {
			return err
		}
	}
	return nil
}

// Function to select an identifying key, to which logseq references can be made
func (b *blockNode) resolveHeadKey() error {
	keyContent, ok := b.Content["key"].([]any)
	if !ok {
		return fmt.Errorf("Something went wrong assigning the head key for: %s", b.Id)
	}
	keyContentLength := len(keyContent)
	if keyContentLength == 0 {
		b.HeadKey = ""
		return nil
	}
	headKey, ok := keyContent[0].(string)
	if ok {
		// if it has a head, check if the block has references
		references, ok := b.Content["references"].([]any)
		if ok && !b.Doc {
			// if it has references, give it a reference tag
			if len(references) > 0 {
				b.HeadKey = strings.Join([]string{"[[", headKey, "]]"}, "")
			}
		} else {
			b.HeadKey = headKey
		}
	} else {
		//("No headkey for %s\n", b.Id)
		b.HeadKey = ""
	}
	return nil
}

func (b *blockNode) parseContent(index *map[string]*blockNode) error {
	// extrapolates parsing to use twice for key and value
	parse := func(key string) (string, error) {
		Content, ok := b.Content[key].([]any)
		if ok != true {
			return "", nil
		}
		ContentLength := len(Content)
		if ContentLength == 0 {
			b.StringedContent = ""
			return "", nil
		}
		totalContent := make([]string, ContentLength)
		for i := 0; i < ContentLength; i++ {
			var appendix string
			switch cell := Content[i].(type) {
			case string:
				appendix = cell
			// reference, image or codeblock
			case map[string]any:
				switch kind := cell["i"].(string); kind {
				// reference
				case "q":
					reference, err := b.parseReference(&cell, index)
					if err != nil {
						return "", err
					}
					appendix = reference
				// image
				case "i":
					image, err := b.parseImage(&cell)
					if err != nil {
						return "", err
					}
					appendix = image
				// codeblock
				case "o":
					codeBlock, err := b.parseCodeBlock(&cell)
					if err != nil {
						return "", err
					}
					appendix = codeBlock
				// markup
				case "m":
					latex, err := b.parseMarkUp(&cell, index)
					if err != nil {
						return "", err
					}
					appendix = latex
				}
				if appendix == "" {
					return "", fmt.Errorf(fmt.Sprintf("Extracting has gone wrong for: %s.\n Cell: %v", b.Id, cell))
				}
			default:
				return "", fmt.Errorf(fmt.Sprintf(key+"-parsing went wrong for %s", b.Id))
			}
			totalContent = append(totalContent, appendix)
		}
		return strings.Join(totalContent, ""), nil
	}
	keyContent, err := parse("key")
	if err != nil {
		return fmt.Errorf("Key Parsing went wrong: %s", err.Error())
	}
	valContent, err := parse("value")
	if err != nil {
		return fmt.Errorf("Val Parsing went wrong: %s", err.Error())
	}
	b.StringedContent = strings.Join([]string{keyContent, valContent}, " ")
	return nil
}

func (b *blockNode) parseReference(cell *map[string]any, index *map[string]*blockNode) (string, error) {
	id, ok := (*cell)["_id"].(string)
	if !ok {
		return "", fmt.Errorf(fmt.Sprintf("Reference but no reference id for: %s", b.Id))
	}
	block, ok := (*index)[id]
	if !ok {
		return " **BROKEN REFERENCE** ", nil
	}
	refHead := block.HeadKey
	if refHead == "" {
		log.Printf("Empty reference from %s to %s.", b.Id, id)
		return "__EMPTY REFERENCE__", nil
	}
	return refHead, nil
}

func (b *blockNode) parseImage(cell *map[string]any) (string, error) {
	url, ok := (*cell)["url"].(string)
	if !ok {
		return "", fmt.Errorf(fmt.Sprintf("Image but no url at %s", b.Id))
	}
	if url == "" {
		return "", fmt.Errorf(fmt.Sprintf("Empty URL for Image at %s", b.Id))
	}
	return strings.Join([]string{"![](", url, ")"}, ""), nil
}

func (b *blockNode) parseCodeBlock(cell *map[string]any) (string, error) {
	text, ok := (*cell)["text"].(string)
	if !ok {
		return "", fmt.Errorf(fmt.Sprintf("Codeblock but no text at %s", b.Id))
	}
	// providing a language is optional. If none is provided, go defaults to ""
	lang, ok := (*cell)["language"].(string)
	return strings.Join([]string{"\n``` " + lang, text, "```"}, "\n"), nil
}

func (b *blockNode) parseMarkUp(cell *map[string]any, index *map[string]*blockNode) (string, error) {
	text, ok := (*cell)["text"].(string)
	var out string
	if !ok {
		return "", fmt.Errorf(fmt.Sprintf("Markup exists, but has no text at %s", b.Id))
	}
	if italics, ok := (*cell)["l"].(bool); italics && ok {
		out = strings.Join([]string{"_", text, "_"}, "")
	}
	if bold, ok := (*cell)["b"].(bool); bold && ok {
		out = strings.Join([]string{"__", text, "__"}, "")
	}
	if underlined, ok := (*cell)["u"].(bool); underlined && ok {
		out = strings.Join([]string{"<ins>", text, "</ins>"}, "")
	}
	if quote, ok := (*cell)["q"].(bool); quote && ok {
		out = strings.Join([]string{"\n  #+BEGIN_QUOTE\n  ", text + "\n", "  #+END_QUOTE"}, "")
	}
	// LaTeX
	if _type, ok := (*cell)["type"].(string); _type == "latex" && ok {
		out = strings.Join([]string{"$$", text, "$$"}, "")
	}
	// Work in Progress
	if wip, ok := (*cell)["workInProgressRem"].(bool); wip && ok {
		out = strings.Join([]string{text, "#WIP"}, " ")
	}
	// Weblink
	if qId, ok := (*cell)["qId"].(string); ok {
		linkBlock, ok := (*index)[qId]
		if !ok {
			log.Printf("Broken link-reference from %s to %s.", b.Id, qId)
			return " **BROKEN LINK REFERENCE** ", nil
		}
		Content, ok := linkBlock.Content["crt"].(map[string]any)
		if !ok {
			return "", fmt.Errorf(fmt.Sprintf("Link reference but no content for %s", linkBlock.Id))
		}
		parentLinkWrapper, ok := Content["b"].(map[string]any)
		if !ok {
			return "", fmt.Errorf(fmt.Sprintf("No parent-link-wrapper for %s", linkBlock.Id))
		}
		childLinkWrapper, ok := parentLinkWrapper["u"].(map[string]any)
		if !ok {
			return "", fmt.Errorf(fmt.Sprintf("No child-link-wrapper for %s", linkBlock.Id))
		}
		link, ok := childLinkWrapper["s"].(string)
		if !ok {
			return "", fmt.Errorf(fmt.Sprintf("No link for %s", linkBlock.Id))
		}
		return strings.Join([]string{"![", linkBlock.HeadKey, "](", link, ")"}, ""), nil
	}
	if out == "" {
		return "", fmt.Errorf(fmt.Sprintf("Something went wrong parsing {%v}-markup at %s with ", (*cell), b.Id))
	}
	return out, nil
}
