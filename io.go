package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

/******************************************************************************

File is structured as so:

Structs:
	AnnotatedSequence - main struct for sequence handling plus sub structs.

File specific parsers, readers, writers, and builders:
	Gff - parser, reader, writer, builder
	Gbk/gb/genbank - parser, reader
	JSON- reader, writer

******************************************************************************/

/******************************************************************************

AnnotatedSequence related structs begin here.

******************************************************************************/

// Meta Holds all the meta information of an AnnotatedSequence struct.
type Meta struct {
	// shared
	Name        string
	GffVersion  string
	RegionStart int
	RegionEnd   int
	// genbank specific
	Size            int
	Type            string
	GenbankDivision string
	Date            string
	Definition      string
	Accession       string
	Version         string
	Keywords        string
	Organism        string
	Source          string
	Origin          string
	Locus           Locus
	References      []Reference
	Primaries       []Primary
}

// Primary Holds all the Primary information of a Meta struct.
type Primary struct {
	RefSeq, PrimaryIdentifier, Primary_Span, Comp string
}

// genbank specific
// type Reference struct {
// 	Authors []string
// 	Title   string
// 	Journal string
// 	PubMed  string
// }

// Reference holds information one reference in a Meta struct.
type Reference struct {
	Index, Authors, Title, Journal, PubMed, Remark, Range string
}

// Locus holds Locus information in a Meta struct.
type Locus struct {
	Name, SequenceLength, MoleculeType, GenBankDivision, ModDate string
	Circular                                                     bool
}

// Feature holds a single annotation in a struct. from https://github.com/blachlylab/gff3/blob/master/gff3.go
type Feature struct {
	Name string //Seqid in gff, name in gbk
	//gff specific
	Source     string
	Type       string
	Start      int
	End        int
	Score      string
	Strand     string
	Phase      string
	Attributes map[string]string // Known as "qualifiers" for gbk, "attributes" for gff.
	//gbk specific
	Location string
	Sequence string
}

// Sequence holds raw sequence information in an AnnotatedSequence struct.
type Sequence struct {
	Description string
	Sequence    string
}

// AnnotatedSequence holds all sequence information in a single struct.
type AnnotatedSequence struct {
	Meta     Meta
	Features []Feature
	Sequence Sequence
}

/******************************************************************************

AnnotatedSequence related structs end here.

******************************************************************************/

/******************************************************************************

GFF specific IO related things begin here.

******************************************************************************/

// ParseGff Takes in a string representing a gffv3 file and parses it into an AnnotatedSequence object.
func ParseGff(gff string) AnnotatedSequence {
	lines := strings.Split(gff, "\n")
	metaString := lines[0:2]
	versionString := metaString[0]
	regionStringArray := strings.Split(metaString[1], " ")

	meta := Meta{}
	meta.GffVersion = strings.Split(versionString, " ")[1]
	meta.Name = regionStringArray[1] // Formally region name, but changed to name here for generality/interoperability.
	meta.RegionStart, _ = strconv.Atoi(regionStringArray[2])
	meta.RegionEnd, _ = strconv.Atoi(regionStringArray[3])
	meta.Size = meta.RegionEnd - meta.RegionStart

	records := []Feature{}
	sequence := Sequence{}
	var sequenceBuffer bytes.Buffer
	fastaFlag := false
	for _, line := range lines {
		if line == "##FASTA" {
			fastaFlag = true
		} else if len(line) == 0 {
			continue
		} else if line[0:2] == "##" {
			continue
		} else if fastaFlag == true && line[0:1] != ">" {
			// sequence.Sequence = sequence.Sequence + line
			sequenceBuffer.WriteString(line)
		} else if fastaFlag == true && line[0:1] == ">" {
			sequence.Description = line
		} else {
			record := Feature{}
			fields := strings.Split(line, "\t")
			record.Name = fields[0]
			record.Source = fields[1]
			record.Type = fields[2]
			record.Start, _ = strconv.Atoi(fields[3])
			record.End, _ = strconv.Atoi(fields[4])
			record.Score = fields[5]
			record.Strand = fields[6]
			record.Phase = fields[7]
			record.Attributes = make(map[string]string)
			attributes := fields[8]
			// var eqIndex int
			attributeSlice := strings.Split(attributes, ";")

			for _, attribute := range attributeSlice {
				attributeSplit := strings.Split(attribute, "=")
				key := attributeSplit[0]
				value := attributeSplit[1]
				record.Attributes[key] = value
			}
			records = append(records, record)
		}
	}
	sequence.Sequence = sequenceBuffer.String()
	annotatedSequence := AnnotatedSequence{}
	annotatedSequence.Meta = meta
	annotatedSequence.Features = records
	annotatedSequence.Sequence = sequence

	return annotatedSequence
}

// BuildGff takes an Annotated sequence and returns a byte array representing a gff to be written out.
func BuildGff(annotatedSequence AnnotatedSequence) []byte {
	var gffBuffer bytes.Buffer

	var versionString string
	if annotatedSequence.Meta.GffVersion != "" {
		versionString = "##gff-version " + annotatedSequence.Meta.GffVersion + "\n"
	} else {
		versionString = "##gff-version 3 \n"
	}
	gffBuffer.WriteString(versionString)

	var regionString string
	var name string
	var start string
	var end string

	if annotatedSequence.Meta.Name != "" {
		name = annotatedSequence.Meta.Name
	} else if annotatedSequence.Meta.Locus.Name != "" {
		name = annotatedSequence.Meta.Locus.Name
	} else if annotatedSequence.Meta.Accession != "" {
		name = annotatedSequence.Meta.Accession
	} else {
		name = "unknown"
	}

	if annotatedSequence.Meta.RegionStart != 0 {
		start = strconv.Itoa(annotatedSequence.Meta.RegionStart)
	} else {
		start = "1"
	}

	if annotatedSequence.Meta.RegionEnd != 0 {
		end = strconv.Itoa(annotatedSequence.Meta.RegionEnd)
	} else if annotatedSequence.Meta.Locus.SequenceLength != "" {
		reg, err := regexp.Compile("[^0-9]+")
		if err != nil {
			log.Fatal(err)
		}
		end = reg.ReplaceAllString(annotatedSequence.Meta.Locus.SequenceLength, "")
	} else {
		end = "1"
	}

	regionString = "##sequence-region " + name + " " + start + " " + end + "\n"
	gffBuffer.WriteString(regionString)

	for _, feature := range annotatedSequence.Features {
		var featureString string

		var featureName string
		if feature.Name != "" {
			featureName = feature.Name
		} else {
			featureName = annotatedSequence.Meta.Name
		}

		var featureSource string
		if feature.Source != "" {
			featureSource = feature.Source
		} else {
			featureSource = "feature"
		}

		var featureType string
		if feature.Type != "" {
			featureType = feature.Type
		} else {
			featureType = "unknown"
		}

		// really really really need to make a genbank parser util for getting start and stop of region.
		featureStart := strconv.Itoa(feature.Start)

		featureEnd := strconv.Itoa(feature.End)
		featureScore := feature.Score
		featureStrand := string(feature.Strand)
		featurePhase := feature.Phase
		var featureAttributes string

		keys := make([]string, 0, len(feature.Attributes))
		for key := range feature.Attributes {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			attributeString := key + "=" + feature.Attributes[key] + ";"
			featureAttributes += attributeString
		}

		if len(featureAttributes) > 0 {
			featureAttributes = featureAttributes[0 : len(featureAttributes)-1]
		}
		TAB := "\t"
		featureString = featureName + TAB + featureSource + TAB + featureType + TAB + featureStart + TAB + featureEnd + TAB + featureScore + TAB + featureStrand + TAB + featurePhase + TAB + featureAttributes + "\n"
		gffBuffer.WriteString(featureString)
	}

	gffBuffer.WriteString("###\n")
	gffBuffer.WriteString("##FASTA\n")
	gffBuffer.WriteString(">" + annotatedSequence.Meta.Name + "\n")

	for letterIndex, letter := range annotatedSequence.Sequence.Sequence {
		letterIndex++
		if letterIndex%70 == 0 && letterIndex != 0 {
			gffBuffer.WriteRune(letter)
			gffBuffer.WriteString("\n")
		} else {
			gffBuffer.WriteRune(letter)
		}
	}
	gffBuffer.WriteString("\n")
	return gffBuffer.Bytes()
}

// ReadGff takes in a filepath for a .gffv3 file and parses it into an Annotated Sequence struct.
func ReadGff(path string) AnnotatedSequence {
	file, err := ioutil.ReadFile(path)
	var annotatedSequence AnnotatedSequence
	if err != nil {
		// return 0, fmt.Errorf("Failed to open file %s for unpack: %s", gzFilePath, err)
	} else {
		annotatedSequence = ParseGff(string(file))
	}
	return annotatedSequence
}

// WriteGff takes an AnnotatedSequence struct and a path string and writes out a gff to that path.
func WriteGff(annotatedSequence AnnotatedSequence, path string) {
	gff := BuildGff(annotatedSequence)
	_ = ioutil.WriteFile(path, gff, 0644)
}

/******************************************************************************

GFF specific IO related things end here.

******************************************************************************/

/******************************************************************************

GBK specific IO related things begin here.

******************************************************************************/

//used in parseLocus function though it could be useful elsewhere.
var genbankDivisions = []string{
	"PRI", //primate sequences
	"ROD", //rodent sequences
	"MAM", //other mamallian sequences
	"VRT", //other vertebrate sequences
	"INV", //invertebrate sequences
	"PLN", //plant, fungal, and algal sequences
	"BCT", //bacterial sequences
	"VRL", //viral sequences
	"PHG", //bacteriophage sequences
	"SYN", //synthetic sequences
	"UNA", //unannotated sequences
	"EST", //EST sequences (expressed sequence tags)
	"PAT", //patent sequences
	"STS", //STS sequences (sequence tagged sites)
	"GSS", //GSS sequences (genome survey sequences)
	"HTG", //HTG sequences (high-throughput genomic sequences)
	"HTC", //unfinished high-throughput cDNA sequencing
	"ENV", //environmental sampling sequences
}

//used in feature check functions.
var genbankTopLevelFeatures = []string{
	"LOCUS",
	"DEFINITION",
	"ACCESSION",
	"VERSION",
	"KEYWORDS",
	"SOURCE",
	"REFERENCE",
	"FEATURES",
	"ORIGIN",
}

//used in feature check functions.
var genbankSubLevelFeatures = []string{
	"ORGANISM",
	"AUTHORS",
	"TITLE",
	"JOURNAL",
	"PUBMED",
	"REMARK",
}

//all gene feature types in genbank
var genbankGeneFeatureTypes = []string{
	"assembly_gap",
	"C_region",
	"CDS",
	"centromere",
	"D-loop",
	"D_segment",
	"exon",
	"gap",
	"gene",
	"iDNA",
	"intron",
	"J_segment",
	"mat_peptide",
	"misc_binding",
	"misc_difference",
	"misc_feature",
	"misc_recomb",
	"misc_RNA",
	"misc_structure",
	"mobile_element",
	"modified_base",
	"mRNA",
	"ncRNA",
	"N_region",
	"old_sequence",
	"operon",
	"oriT",
	"polyA_site",
	"precursor_RNA",
	"prim_transcript",
	"primer_bind",
	"propeptide",
	"protein_bind",
	"regulatory",
	"repeat_region",
	"rep_origin",
	"rRNA",
	"S_region",
	"sig_peptide",
	"source",
	"stem_loop",
	"STS",
	"telomere",
	"tmRNA",
	"transit_peptide",
	"tRNA",
	"unsure",
	"V_region",
	"V_segment",
	"variation",
	"3'UTR",
	"5'UTR",
}

// all genbank feature qualifiers
var genbankGeneQualifierTypes = []string{
	"/allele=",
	"/altitude=",
	"/anticodon=",
	"/artificial_location",
	"/bio_material=",
	"/bound_moiety=",
	"/cell_line=",
	"/cell_type=",
	"/chromosome=",
	"/citation=",
	"/clone=",
	"/clone_lib=",
	"/codon_start=",
	"/collected_by=",
	"/collection_date=",
	"/compare=",
	"/country=",
	"/cultivar=",
	"/culture_collection=",
	"/db_xref=",
	"/dev_stage=",
	"/direction=",
	"/EC_number=",
	"/ecotype=",
	"/environmental_sample",
	"/estimated_length=",
	"/exception=",
	"/experiment=",
	"/focus",
	"/frequency=",
	"/function=",
	"/gap_type=",
	"/gene=",
	"/gene_synonym=",
	"/germline",
	"/haplogroup=",
	"/haplotype=",
	"/host=",
	"/identified_by=",
	"/inference=",
	"/isolate=",
	"/isolation_source=",
	"/lab_host=",
	"/lat_lon=",
	"/linkage_evidence=",
	"/locus_tag=",
	"/macronuclear",
	"/map=",
	"/mating_type=",
	"/metagenome_source",
	"/mobile_element_type=",
	"/mod_base=",
	"/mol_type=",
	"/ncRNA_class=",
	"/note=",
	"/number=",
	"/old_locus_tag=",
	"/operon=",
	"/organelle=",
	"/organism=",
	"/partial",
	"/PCR_conditions=",
	"/PCR_primers=",
	"/phenotype=",
	"/plasmid=",
	"/pop_variant=",
	"/product=",
	"/protein_id=",
	"/proviral",
	"/pseudo",
	"/pseudogene=",
	"/rearranged",
	"/replace=",
	"/ribosomal_slippage",
	"/rpt_family=",
	"/rpt_type=",
	"/rpt_unit_range=",
	"/rpt_unit_seq=",
	"/satellite=",
	"/segment=",
	"/serotype=",
	"/serovar=",
	"/sex=",
	"/specimen_voucher=",
	"/standard_name=",
	"/strain=",
	"/sub_clone=",
	"/submitter_seqid=",
	"/sub_species=",
	"/sub_strain=",
	"/tag_peptide=",
	"/tissue_lib=",
	"/tissue_type=",
	"/transgenic",
	"/translation=",
	"/transl_except=",
	"/transl_table=",
	"/trans_splicing",
	"/type_material=",
	"/variety=",
}

// indeces for random points of interests on a gbk line.
const metaIndex = 0
const subMetaIndex = 5
const qualifierIndex = 21

func quickMetaCheck(line string) bool {
	flag := false
	if string(line[metaIndex]) != " " {
		flag = true
	}
	return flag
}

func quickSubMetaCheck(line string) bool {
	flag := false

	if string(line[metaIndex]) == " " && string(line[subMetaIndex]) != " " {
		flag = true
	}
	return flag
}

func quickFeatureCheck(line string) bool {
	flag := false

	if string(line[metaIndex]) == " " && string(line[subMetaIndex]) != " " {
		flag = true
	}
	return flag
}

func quickQualifierCheck(line string) bool {
	flag := false

	if string(line[metaIndex]) == " " && string(line[subMetaIndex]) == " " && string(line[qualifierIndex]) == "/" {
		flag = true
	}
	return flag

}

func quickQualifierSubLineCheck(line string) bool {
	flag := false

	if string(line[metaIndex]) == " " && string(line[subMetaIndex]) == " " && string(line[qualifierIndex]) != "/" && string(line[qualifierIndex-1]) == " " {
		flag = true
	}
	return flag
}

// checks for only top level features in genbankTopLevelFeatures array
func topLevelFeatureCheck(featureString string) bool {
	flag := false
	cleanedFeatureString := strings.TrimSpace(featureString)
	for _, feature := range genbankTopLevelFeatures {
		if feature == cleanedFeatureString {
			flag = true
			break
		}
	}
	return flag
}

// checks for only sub level features in genbankSubLevelFeatures array
func subLevelFeatureCheck(featureString string) bool {
	flag := false
	cleanedFeatureString := strings.TrimSpace(featureString)
	for _, feature := range genbankSubLevelFeatures {
		if feature == cleanedFeatureString {
			flag = true
			break
		}
	}
	return flag
}

// checks for both sub and top level features in genbankSubLevelFeatures and genbankTopLevelFeatures array
func allLevelFeatureCheck(featureString string) bool {
	flag := false
	cleanedFeatureString := strings.TrimSpace(featureString)
	if subLevelFeatureCheck(cleanedFeatureString) || topLevelFeatureCheck(cleanedFeatureString) {
		flag = true
	}
	return flag
}

// will eventually refactor all checks into one function.
func geneFeatureTypeCheck(featureString string) bool {
	flag := false
	cleanedFeatureString := strings.TrimSpace(featureString)
	for _, feature := range genbankGeneFeatureTypes {
		if feature == cleanedFeatureString {
			flag = true
			break
		}
	}
	return flag
}

func geneQualifierTypeCheck(featureString string) bool {
	flag := false
	cleanedFeatureString := strings.TrimSpace(strings.SplitAfter(featureString, "=")[0])
	for _, feature := range genbankGeneQualifierTypes {
		if feature == cleanedFeatureString {
			flag = true
			break
		}
	}
	return flag
}

func allGeneTypeCheck(featureString string) bool {
	flag := false
	cleanedFeatureString := strings.TrimSpace(featureString)
	if geneQualifierTypeCheck(cleanedFeatureString) || topLevelFeatureCheck(cleanedFeatureString) {
		flag = true
	}
	return flag
}

// parses locus from provided string.
func parseLocus(locusString string) Locus {
	locus := Locus{}
	locusSplit := strings.Split(strings.TrimSpace(locusString), " ")
	var filteredLocusSplit []string
	for i := range locusSplit {
		if locusSplit[i] != "" {
			filteredLocusSplit = append(filteredLocusSplit, locusSplit[i])
		}
	}
	locus.Name = filteredLocusSplit[1]
	locus.SequenceLength = strings.Join([]string{filteredLocusSplit[2], filteredLocusSplit[3]}, " ")
	locus.MoleculeType = filteredLocusSplit[4]
	if filteredLocusSplit[5] == "circular" || filteredLocusSplit[5] == "linear" {
		if filteredLocusSplit[5] == "circular" {
			locus.Circular = true
		} else {
			locus.Circular = false
		}
		locus.GenBankDivision = filteredLocusSplit[6]
		locus.ModDate = filteredLocusSplit[7]
	} else {
		locus.Circular = false
		locus.GenBankDivision = filteredLocusSplit[5]
		locus.ModDate = filteredLocusSplit[6]
	}
	return locus
}

// really important helper function. It finds sublines of a feature and joins them.
func joinSubLines(splitLine, subLines []string) string {
	base := strings.TrimSpace(strings.Join(splitLine[1:], " "))

	for _, subLine := range subLines {
		if !quickMetaCheck(subLine) && !quickSubMetaCheck(subLine) {
			base = strings.TrimSpace(strings.TrimSpace(base) + " " + strings.TrimSpace(subLine))
		} else {
			break
		}
	}
	return base
}

// get organism name and source. Doesn't use joinSubLines.
func getSourceOrganism(splitLine, subLines []string) (string, string) {
	source := strings.TrimSpace(strings.Join(splitLine[1:], " "))
	var organism string
	for numSubLine, subLine := range subLines {
		headString := strings.Split(strings.TrimSpace(subLine), " ")[0]
		if string(subLine[0]) == " " && headString != "ORGANISM" {
			source = strings.TrimSpace(strings.TrimSpace(source) + " " + strings.TrimSpace(subLine))
		} else {
			organismSubLines := subLines[numSubLine+1:]
			organismSplitLine := strings.Split(strings.TrimSpace(subLine), " ")
			organism = joinSubLines(organismSplitLine, organismSubLines)
			break
		}
	}
	return source, organism
}

// gets a single reference. Parses headstring and the joins sub lines based on feature.
func getReference(splitLine, subLines []string) Reference {
	base := strings.TrimSpace(strings.Join(splitLine[1:], " "))
	reference := Reference{}
	reference.Index = strings.Split(base, " ")[0]
	if len(base) > 1 {
		reference.Range = strings.TrimSpace(strings.Join(strings.Split(base, " ")[1:], " "))
	}

	for numSubLine, subLine := range subLines {
		featureSubLines := subLines[numSubLine+1:]
		featureSplitLine := strings.Split(strings.TrimSpace(subLine), " ")
		headString := featureSplitLine[0]
		if topLevelFeatureCheck(headString) {
			break
		}
		switch headString {
		case "AUTHORS":
			reference.Authors = joinSubLines(featureSplitLine, featureSubLines)
		case "TITLE":
			reference.Title = joinSubLines(featureSplitLine, featureSubLines)
		case "JOURNAL":
			reference.Journal = joinSubLines(featureSplitLine, featureSubLines)
		case "PUBMED":
			reference.PubMed = joinSubLines(featureSplitLine, featureSubLines)
		case "REMARK":
			reference.Remark = joinSubLines(featureSplitLine, featureSubLines)
		default:
			break
		}

	}
	return reference
}

func getFeatures(lines []string) []Feature {
	lineIndex := 0
	features := []Feature{}

	// regex to remove quotes and slashes from qualifiers
	reg, _ := regexp.Compile("[\"/\n]+")

	// go through every line.
	for lineIndex < len(lines) {
		line := lines[lineIndex]
		// This is a break to ensure that cursor doesn't go beyond ORIGIN which is the last top level feature.
		// This could pick up random sequence strings that aren't helpful and will mess with parser.
		// DO NOT MOVE/REMOVE WITHOUT CAUSE AND CONSIDERATION
		if quickMetaCheck(line) || !quickFeatureCheck(line) {
			break
		}

		feature := Feature{}

		// split the current line for feature type and location fields.
		splitLine := strings.Split(strings.TrimSpace(line), " ")

		// assign type and location to feature.
		feature.Type = strings.TrimSpace(splitLine[0])
		feature.Location = strings.TrimSpace(splitLine[len(splitLine)-1])

		// initialize attributes.
		feature.Attributes = make(map[string]string)

		// end of feature declaration line. Bump to next line and begin looking for qualifiers.
		lineIndex++
		line = lines[lineIndex]

		// loop through potential qualifiers. Break if not a qualifier or sub line.
		// Definition of qualifiers here: http://www.insdc.org/files/feature_table.html#3.3
		for {
			// make sure what we're parsing is a qualifier. Break if not.
			// keeping out of normal if else pattern because of phantom brackets that are hard to trace.
			if !quickQualifierCheck(line) {
				break
			}
			qualifier := line

			// end of qualifier declaration line. Bump to next line and begin looking for qualifier sublines.
			lineIndex++
			line = lines[lineIndex]

			// loop through any potential continuing lines of qualifiers. Break if not.
			for {
				// keeping out of normal if else pattern because of phantom brackets that are hard to trace.
				if !quickQualifierSubLineCheck(line) {
					break
				}
				//append to current qualifier
				qualifier += strings.TrimSpace(line)

				// nextline
				lineIndex++
				line = lines[lineIndex]
			}
			//add qualifier to feature.
			attributeSplit := strings.Split(reg.ReplaceAllString(qualifier, ""), "=")
			attributeLabel := strings.TrimSpace(attributeSplit[0])
			var attributeValue string
			if len(attributeSplit) < 2 {
				attributeValue = ""
			} else {
				attributeValue = strings.TrimSpace(attributeSplit[1])
			}
			feature.Attributes[attributeLabel] = attributeValue
		}

		//append the parsed feature to the features list to be returned.
		features = append(features, feature)

	}
	return features
}

// takes every line after origin feature and removes anything that isn't in the alphabet. Returns sequence.
func getSequence(subLines []string) Sequence {
	sequence := Sequence{}
	var sequenceBuffer bytes.Buffer
	reg, err := regexp.Compile("[^a-zA-Z]+")
	if err != nil {
		log.Fatal(err)
	}
	for _, subLine := range subLines {
		sequenceBuffer.WriteString(subLine)
	}
	sequence.Sequence = reg.ReplaceAllString(sequenceBuffer.String(), "")
	return sequence
}

// ParseGbk takes in a string representing a gbk/gb/genbank file and parses it into an AnnotatedSequence object.
func ParseGbk(gbk string) AnnotatedSequence {

	lines := strings.Split(gbk, "\n")

	// Create meta struct
	meta := Meta{}

	// Create features struct
	features := []Feature{}

	// Create sequence struct
	sequence := Sequence{}

	for numLine := 0; numLine < len(lines); numLine++ {
		line := lines[numLine]
		splitLine := strings.Split(line, " ")
		subLines := lines[numLine+1:]

		// This is to keep the cursor from scrolling to the bottom another time after getSequence() is called.
		// Break has to be in scope and can't be called within switch statement.
		// Otherwise it will just break the switch which is redundant.
		sequenceBreakFlag := false
		if sequenceBreakFlag == true {
			break
		}

		switch splitLine[0] {

		case "":
			continue
		case "LOCUS":
			meta.Locus = parseLocus(line)
		case "DEFINITION":
			meta.Definition = joinSubLines(splitLine, subLines)
		case "ACCESSION":
			meta.Accession = joinSubLines(splitLine, subLines)
		case "VERSION":
			meta.Version = joinSubLines(splitLine, subLines)
		case "KEYWORDS":
			meta.Keywords = joinSubLines(splitLine, subLines)
		case "SOURCE":
			meta.Source, meta.Organism = getSourceOrganism(splitLine, subLines)
		case "REFERENCE":
			meta.References = append(meta.References, getReference(splitLine, subLines))
			continue
		case "FEATURES":
			features = getFeatures(subLines)
		case "ORIGIN":
			sequence = getSequence(subLines)
			sequenceBreakFlag = true
		default:
			continue
		}

	}
	var annotatedSequence AnnotatedSequence
	annotatedSequence.Meta = meta
	annotatedSequence.Features = features
	annotatedSequence.Sequence = sequence

	return annotatedSequence
}

// ReadGbk reads a Gbk from path and parses into an Annotated sequence struct.
func ReadGbk(path string) AnnotatedSequence {
	file, err := ioutil.ReadFile(path)
	var annotatedSequence AnnotatedSequence
	if err != nil {
		// return 0, fmt.Errorf("Failed to open file %s for unpack: %s", gzFilePath, err)
	} else {
		gbkString := string(file)
		annotatedSequence = ParseGbk(gbkString)

	}
	return annotatedSequence
}

/******************************************************************************

GBK specific IO related things end here.

******************************************************************************/

/******************************************************************************

JSON specific IO related things begin here.

******************************************************************************/

// WriteJSON writes an AnnotatedSequence struct out to json.
func WriteJSON(annotatedSequence AnnotatedSequence, path string) {
	file, _ := json.MarshalIndent(annotatedSequence, "", " ")
	_ = ioutil.WriteFile(path, file, 0644)
}

// ReadJSON reads an AnnotatedSequence JSON file.
func ReadJSON(path string) AnnotatedSequence {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		// return 0, fmt.Errorf("Failed to open file %s for unpack: %s", gzFilePath, err)
	}
	var annotatedSequence AnnotatedSequence
	json.Unmarshal([]byte(file), &annotatedSequence)
	return annotatedSequence
}

/******************************************************************************

JSON specific IO related things end here.

******************************************************************************/
